//! UDS (Unix Domain Socket) server — binary-framed transport.
//!
//! Phase 1 is intentionally simple:
//!   - Single-threaded tokio runtime (workspace Cargo.toml picks only `rt`,
//!     not `rt-multi-thread`). Ref:
//!     https://docs.rs/tokio/latest/tokio/runtime/struct.Builder.html#method.new_current_thread
//!   - One task per accepted connection.
//!   - Framing: 4-byte BE outer length + binary body, body opens with a
//!     1-byte opcode. See `protocol.rs`.
//!   - Request kinds: `parse_npm_lockfile`, `parse_gomod_lockfile`, `ping`.
//!
//! Platform scope: Unix only (`tokio::net::UnixListener` requires `cfg(unix)`
//! per https://docs.rs/tokio/latest/tokio/net/struct.UnixListener.html).
//! Windows named-pipe support is Phase 2 — see ADR-0005.

use std::path::{Path, PathBuf};
use std::time::{Duration, Instant};

use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{UnixListener, UnixStream};
use tokio::sync::oneshot;
use tracing::{debug, error, info, warn};

use crate::parsers::{gomod, npm, ParseError, ParsedSbom};
use crate::protocol::{
    decode_request_body, encode_error, encode_parse_result, encode_pong, Request, MAX_FRAME_BYTES,
};

/// Guard against slowloris-style hangs: once a frame has started, it
/// must finish within this window or the connection is dropped. We
/// do NOT apply this to the first read (idle connection waiting for
/// a new request is legitimate).
const FRAME_READ_TIMEOUT: Duration = Duration::from_secs(30);

/// Bind the UDS listener at `path` and run until the caller drops
/// `shutdown`. Removes any stale socket file before binding.
pub async fn serve(socket_path: impl AsRef<Path>) -> Result<ServerHandle, std::io::Error> {
    let path: PathBuf = socket_path.as_ref().to_path_buf();
    match std::fs::remove_file(&path) {
        Ok(_) => debug!(?path, "removed stale socket"),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
        Err(e) => return Err(e),
    }

    let listener = UnixListener::bind(&path)?;
    info!(
        ?path,
        max_frame_bytes = MAX_FRAME_BYTES,
        "rampart-native listening on UDS (binary envelope)"
    );

    let (shutdown_tx, shutdown_rx) = oneshot::channel::<()>();
    let accept_path = path.clone();
    let accept_loop = tokio::spawn(async move {
        tokio::pin!(shutdown_rx);
        loop {
            tokio::select! {
                _ = &mut shutdown_rx => {
                    info!("shutdown signalled; stopping accept loop");
                    break;
                }
                accept = listener.accept() => {
                    match accept {
                        Ok((stream, _)) => {
                            tokio::spawn(handle_connection(stream));
                        }
                        Err(e) => {
                            error!(error = %e, "accept failed; continuing");
                        }
                    }
                }
            }
        }
        let _ = std::fs::remove_file(&accept_path);
    });

    Ok(ServerHandle {
        path,
        shutdown: Some(shutdown_tx),
        join: Some(accept_loop),
    })
}

/// Handle to a running server. Drop (or call [`ServerHandle::shutdown`])
/// to stop.
pub struct ServerHandle {
    pub path: PathBuf,
    shutdown: Option<oneshot::Sender<()>>,
    join: Option<tokio::task::JoinHandle<()>>,
}

impl ServerHandle {
    pub async fn shutdown(mut self) {
        if let Some(tx) = self.shutdown.take() {
            let _ = tx.send(());
        }
        if let Some(h) = self.join.take() {
            let _ = h.await;
        }
    }
}

impl Drop for ServerHandle {
    fn drop(&mut self) {
        if let Some(tx) = self.shutdown.take() {
            let _ = tx.send(());
        }
    }
}

async fn handle_connection(mut stream: UnixStream) {
    loop {
        match read_frame(&mut stream).await {
            Ok(None) => {
                debug!("client closed connection");
                return;
            }
            Ok(Some(body)) => {
                let resp = handle_body(&body);
                if let Err(e) = stream.write_all(&resp).await {
                    warn!(error = %e, "write failed; closing connection");
                    return;
                }
                if let Err(e) = stream.flush().await {
                    warn!(error = %e, "flush failed; closing connection");
                    return;
                }
            }
            Err(e) => {
                warn!(error = %e, "read failed; closing connection");
                return;
            }
        }
    }
}

/// Read one frame body. Returns `Ok(None)` on clean client close,
/// `Err` on protocol violation or timeout.
async fn read_frame(stream: &mut UnixStream) -> Result<Option<Vec<u8>>, std::io::Error> {
    // Waiting for the NEXT frame is legitimate — no timeout here.
    // A tokio read_exact resolves to UnexpectedEof when the client
    // closes before any bytes arrive; that's the clean-close signal.
    let mut len_buf = [0u8; 4];
    match stream.read_exact(&mut len_buf).await {
        Ok(_) => {}
        Err(e) if e.kind() == std::io::ErrorKind::UnexpectedEof => return Ok(None),
        Err(e) => return Err(e),
    }
    let len = u32::from_be_bytes(len_buf);
    if len == 0 || len > MAX_FRAME_BYTES {
        return Err(std::io::Error::new(
            std::io::ErrorKind::InvalidData,
            format!("frame length {len} out of range [1, {MAX_FRAME_BYTES}]"),
        ));
    }
    // Within a frame: bound the read time so a trickling client can't
    // hold the server task forever.
    let mut body = vec![0u8; len as usize];
    read_exact_with_timeout(stream, &mut body).await?;
    Ok(Some(body))
}

async fn read_exact_with_timeout(
    stream: &mut UnixStream,
    buf: &mut [u8],
) -> Result<(), std::io::Error> {
    match tokio::time::timeout(FRAME_READ_TIMEOUT, stream.read_exact(buf)).await {
        Ok(Ok(_)) => Ok(()),
        Ok(Err(e)) => Err(e),
        Err(_) => Err(std::io::Error::new(
            std::io::ErrorKind::TimedOut,
            "frame body read timed out",
        )),
    }
}

fn handle_body(body: &[u8]) -> Vec<u8> {
    let req = match decode_request_body(body) {
        Ok(r) => r,
        Err(e) => {
            return encode_error("MALFORMED_REQUEST", &format!("request decode failed: {e}"));
        }
    };
    match req {
        Request::Ping => encode_pong(),
        Request::ParseNpm {
            content,
            reserved_metadata: _,
        } => dispatch_parse("npm", npm::parse(&content), content.len()),
        Request::ParseGomod { gosum, gomod } => {
            dispatch_parse("gomod", gomod::parse(&gosum, &gomod), gosum.len() + gomod.len())
        }
    }
}

fn dispatch_parse(
    eco: &'static str,
    result: Result<ParsedSbom, ParseError>,
    bytes_read: usize,
) -> Vec<u8> {
    let start = Instant::now();
    match result {
        Ok(parsed_sbom) => {
            debug!(
                ecosystem = eco,
                parse_ms = start.elapsed().as_millis() as u64,
                package_count = parsed_sbom.packages.len(),
                bytes_read,
                "parse ok"
            );
            let json = match serde_json::to_vec(&parsed_sbom) {
                Ok(b) => b,
                Err(e) => {
                    return encode_error("INTERNAL_ERROR", &format!("sbom marshal failed: {e}"));
                }
            };
            encode_parse_result(&json)
        }
        Err(e) => {
            let code = match &e {
                ParseError::Malformed(_) => "MALFORMED_LOCKFILE",
                ParseError::UnsupportedVersion(_) => "UNSUPPORTED_LOCKFILE_VERSION",
                ParseError::Empty(_) => "EMPTY_LOCKFILE",
            };
            encode_error(code, &e.to_string())
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::protocol::{
        MSG_ERROR, MSG_PARSE_GOMOD_REQUEST, MSG_PARSE_REQUEST, MSG_PARSE_RESULT, MSG_PING,
        MSG_PONG,
    };

    fn build_parse_request_body(content: &[u8]) -> Vec<u8> {
        let mut body = Vec::new();
        body.push(MSG_PARSE_REQUEST);
        body.extend_from_slice(&(content.len() as u32).to_be_bytes());
        body.extend_from_slice(content);
        body.extend_from_slice(&0u32.to_be_bytes()); // metadata_len = 0
        body
    }

    #[test]
    fn handle_ping_returns_pong() {
        let resp = handle_body(&[MSG_PING]);
        // outer_len=1 prefix, then MSG_PONG
        assert_eq!(resp, vec![0, 0, 0, 1, MSG_PONG]);
    }

    #[test]
    fn handle_parse_v3_lockfile() {
        let body = br#"{"lockfileVersion":3,"packages":{"": {"name":"x","version":"1"}}}"#;
        let req_body = build_parse_request_body(body);
        let resp = handle_body(&req_body);

        // Parse the outer layout of the response.
        assert!(resp.len() > 5);
        let outer_len = u32::from_be_bytes([resp[0], resp[1], resp[2], resp[3]]) as usize;
        assert_eq!(resp.len(), 4 + outer_len);
        assert_eq!(resp[4], MSG_PARSE_RESULT);
        let sbom_len = u32::from_be_bytes([resp[5], resp[6], resp[7], resp[8]]) as usize;
        assert_eq!(sbom_len, resp.len() - 9);

        // The JSON must be a valid ParsedSBOM with zero packages.
        let json = &resp[9..];
        let v: serde_json::Value = serde_json::from_slice(json).unwrap();
        assert_eq!(v["Ecosystem"], "npm");
        assert_eq!(v["Packages"].as_array().unwrap().len(), 0);
    }

    #[test]
    fn handle_parse_v2_returns_error_frame() {
        let body = br#"{"lockfileVersion":2,"packages":{}}"#;
        let req_body = build_parse_request_body(body);
        let resp = handle_body(&req_body);
        assert_eq!(resp[4], MSG_ERROR);
        let code_len = u32::from_be_bytes([resp[5], resp[6], resp[7], resp[8]]) as usize;
        let code = &resp[9..9 + code_len];
        assert_eq!(code, b"UNSUPPORTED_LOCKFILE_VERSION");
    }

    #[test]
    fn handle_unknown_opcode_returns_malformed_error() {
        let resp = handle_body(&[0x7A]);
        assert_eq!(resp[4], MSG_ERROR);
        let code_len = u32::from_be_bytes([resp[5], resp[6], resp[7], resp[8]]) as usize;
        let code = &resp[9..9 + code_len];
        assert_eq!(code, b"MALFORMED_REQUEST");
    }

    #[test]
    fn handle_empty_body_returns_malformed_error() {
        let resp = handle_body(&[]);
        assert_eq!(resp[4], MSG_ERROR);
    }

    #[test]
    fn handle_parse_gomod_returns_result() {
        let gosum = b"github.com/x/y v1.0.0 h1:hh=\n";
        let gomod = b"module example.com/x\n";
        let mut body = Vec::new();
        body.push(MSG_PARSE_GOMOD_REQUEST);
        body.extend_from_slice(&(gosum.len() as u32).to_be_bytes());
        body.extend_from_slice(gosum);
        body.extend_from_slice(&(gomod.len() as u32).to_be_bytes());
        body.extend_from_slice(gomod);
        let resp = handle_body(&body);
        assert_eq!(resp[4], MSG_PARSE_RESULT);
        let sbom_len = u32::from_be_bytes([resp[5], resp[6], resp[7], resp[8]]) as usize;
        let json = &resp[9..9 + sbom_len];
        let v: serde_json::Value = serde_json::from_slice(json).unwrap();
        assert_eq!(v["Ecosystem"], "gomod");
        assert_eq!(v["Packages"].as_array().unwrap().len(), 1);
    }

}
