//! UDS (Unix Domain Socket) server.
//!
//! Phase 1 is intentionally simple:
//!   - Single-threaded tokio runtime (workspace Cargo.toml picks only `rt`,
//!     not `rt-multi-thread`). Ref:
//!     https://docs.rs/tokio/latest/tokio/runtime/struct.Builder.html#method.new_current_thread
//!   - One task per accepted connection.
//!   - Framing: 4-byte big-endian u32 length prefix + JSON body.
//!   - Three request kinds: `parse_npm_lockfile`, `ping`, `shutdown`.
//!
//! Platform scope: Unix only (`tokio::net::UnixListener` requires `cfg(unix)`
//! per https://docs.rs/tokio/latest/tokio/net/struct.UnixListener.html).
//! Windows named-pipe support is Phase 2 — see ADR-0005.

use std::path::{Path, PathBuf};
use std::time::Instant;

use base64::prelude::*;
use chrono::{SecondsFormat, Utc};
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{UnixListener, UnixStream};
use tokio::sync::oneshot;
use tracing::{debug, error, info, warn};

use crate::parser::{parse, Meta};
use crate::protocol::{ParseResult, ParseStats, Request, RequestPayload, Response, ResponseError};

/// Maximum request frame size the server accepts: 100 MiB. A huge
/// package-lock.json is ~50 MiB (FIRST.md benchmark fixture), so 100 MiB
/// leaves a safety margin and hard-caps the cost of a malformed length
/// prefix.
const MAX_FRAME_BYTES: u32 = 100 * 1024 * 1024;

/// Bind the UDS listener at `path` and run until the caller drops
/// `shutdown`. Removes any stale socket file before binding.
///
/// Returns a tuple of `(listener, shutdown_tx)` — caller drops or sends
/// on `shutdown_tx` to stop accepting new connections. Returns the path
/// actually bound so the caller (or tests) can display it.
pub async fn serve(socket_path: impl AsRef<Path>) -> Result<ServerHandle, std::io::Error> {
    let path: PathBuf = socket_path.as_ref().to_path_buf();
    // Clean up stale socket from a prior crash — bind would otherwise
    // fail with EADDRINUSE.
    match std::fs::remove_file(&path) {
        Ok(_) => debug!(?path, "removed stale socket"),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
        Err(e) => return Err(e),
    }

    let listener = UnixListener::bind(&path)?;
    info!(
        ?path,
        max_frame_bytes = MAX_FRAME_BYTES,
        "rampart-native listening on UDS"
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
        // Best-effort cleanup on shutdown so we don't leave a socket file behind.
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

/// Per-connection driver loop. Reads length-prefixed JSON requests until
/// the client half-closes or we hit a fatal framing error.
async fn handle_connection(mut stream: UnixStream) {
    loop {
        match read_frame(&mut stream).await {
            Ok(None) => {
                debug!("client closed connection");
                return;
            }
            Ok(Some(bytes)) => {
                let resp_bytes = handle_request(&bytes);
                if let Err(e) = write_frame(&mut stream, &resp_bytes).await {
                    warn!(error = %e, "write failed; closing connection");
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

async fn read_frame(stream: &mut UnixStream) -> Result<Option<Vec<u8>>, std::io::Error> {
    let mut len_buf = [0u8; 4];
    match stream.read_exact(&mut len_buf).await {
        Ok(_) => {}
        Err(e) if e.kind() == std::io::ErrorKind::UnexpectedEof => return Ok(None),
        Err(e) => return Err(e),
    }
    let len = u32::from_be_bytes(len_buf);
    if len > MAX_FRAME_BYTES {
        return Err(std::io::Error::new(
            std::io::ErrorKind::InvalidData,
            format!("frame size {len} exceeds MAX_FRAME_BYTES ({MAX_FRAME_BYTES})"),
        ));
    }
    let mut body = vec![0u8; len as usize];
    stream.read_exact(&mut body).await?;
    Ok(Some(body))
}

async fn write_frame(stream: &mut UnixStream, body: &[u8]) -> Result<(), std::io::Error> {
    let len = body.len();
    if len > MAX_FRAME_BYTES as usize {
        return Err(std::io::Error::new(
            std::io::ErrorKind::InvalidData,
            "response exceeds MAX_FRAME_BYTES",
        ));
    }
    let len_buf = (len as u32).to_be_bytes();
    stream.write_all(&len_buf).await?;
    stream.write_all(body).await?;
    stream.flush().await?;
    Ok(())
}

fn handle_request(bytes: &[u8]) -> Vec<u8> {
    let req: Request = match serde_json::from_slice(bytes) {
        Ok(r) => r,
        Err(e) => {
            let resp = Response {
                id: "unknown".to_string(),
                kind: "error".to_string(),
                payload: None,
                error: Some(ResponseError {
                    code: "MALFORMED_REQUEST".to_string(),
                    message: format!("request JSON decode failed: {e}"),
                }),
            };
            return serde_json::to_vec(&resp).unwrap_or_default();
        }
    };
    let resp = match req.kind.as_str() {
        "ping" => Response {
            id: req.id,
            kind: "pong".to_string(),
            payload: Some(serde_json::json!({})),
            error: None,
        },
        "shutdown" => {
            // Server stays up by default; shutdown is handled at the
            // process level (SIGTERM). Acknowledge politely so the
            // client never gets stuck.
            Response {
                id: req.id,
                kind: "shutdown_ack".to_string(),
                payload: Some(serde_json::json!({})),
                error: None,
            }
        }
        "parse_npm_lockfile" => handle_parse_npm(req),
        other => Response {
            id: req.id,
            kind: "error".to_string(),
            payload: None,
            error: Some(ResponseError {
                code: "UNKNOWN_REQUEST".to_string(),
                message: format!("unknown request type `{other}`"),
            }),
        },
    };
    serde_json::to_vec(&resp).unwrap_or_default()
}

fn handle_parse_npm(req: Request) -> Response {
    let Some(RequestPayload::ParseNpmLockfile(p)) = req.payload else {
        return Response {
            id: req.id,
            kind: "error".to_string(),
            payload: None,
            error: Some(ResponseError {
                code: "MALFORMED_REQUEST".to_string(),
                message: "parse_npm_lockfile requires `payload.content`".to_string(),
            }),
        };
    };
    let content_bytes = match BASE64_STANDARD.decode(p.content.as_bytes()) {
        Ok(b) => b,
        Err(e) => {
            return Response {
                id: req.id,
                kind: "error".to_string(),
                payload: None,
                error: Some(ResponseError {
                    code: "INVALID_BASE64".to_string(),
                    message: format!("payload.content is not valid base64: {e}"),
                }),
            };
        }
    };
    // Fill GeneratedAt with UTC now if the caller left it blank —
    // mirrors the Go parser's behaviour and keeps the SBOM JSON
    // unmarshallable on the Go side (time.Time refuses empty strings;
    // RFC3339Nano here matches Go's time.RFC3339Nano format).
    let generated_at = match p.generated_at {
        Some(s) if !s.is_empty() => s,
        _ => Utc::now().to_rfc3339_opts(SecondsFormat::Nanos, true),
    };
    // Same for ID: fall back to a stable prefix + nanos. Adım 7 wires
    // a real ULID if callers need one in production; parity tests
    // always pass an explicit id, so this branch is benchmark-only.
    let id = match p.id {
        Some(s) if !s.is_empty() => s,
        _ => format!(
            "rampart-native-{}",
            Utc::now().timestamp_nanos_opt().unwrap_or_default()
        ),
    };
    let meta = Meta {
        component_ref: p.component_ref.unwrap_or_default(),
        commit_sha: p.commit_sha.unwrap_or_default(),
        generated_at,
        id,
    };
    let start = Instant::now();
    match parse(&content_bytes, &meta) {
        Ok(sbom) => {
            let elapsed = start.elapsed();
            let stats = ParseStats {
                parse_ms: elapsed.as_millis() as u64,
                package_count: sbom.packages.len(),
                bytes_read: content_bytes.len(),
            };
            let result = ParseResult { sbom, stats };
            let payload = serde_json::to_value(&result).unwrap_or_default();
            Response {
                id: req.id,
                kind: "parse_result".to_string(),
                payload: Some(payload),
                error: None,
            }
        }
        Err(e) => {
            let code = match &e {
                crate::parser::ParseError::Malformed(_) => "MALFORMED_LOCKFILE",
                crate::parser::ParseError::UnsupportedVersion(_) => "UNSUPPORTED_LOCKFILE_VERSION",
                crate::parser::ParseError::Empty => "EMPTY_LOCKFILE",
            };
            Response {
                id: req.id,
                kind: "error".to_string(),
                payload: None,
                error: Some(ResponseError {
                    code: code.to_string(),
                    message: e.to_string(),
                }),
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn handle_ping_returns_pong() {
        let req = br#"{"id":"p1","type":"ping"}"#;
        let resp = handle_request(req);
        let v: serde_json::Value = serde_json::from_slice(&resp).unwrap();
        assert_eq!(v["id"], "p1");
        assert_eq!(v["type"], "pong");
        assert!(v.get("error").is_none());
    }

    #[test]
    fn handle_unknown_kind() {
        let req = br#"{"id":"u1","type":"quuz"}"#;
        let resp = handle_request(req);
        let v: serde_json::Value = serde_json::from_slice(&resp).unwrap();
        assert_eq!(v["type"], "error");
        assert_eq!(v["error"]["code"], "UNKNOWN_REQUEST");
    }

    #[test]
    fn handle_malformed_request() {
        let req = b"{not json";
        let resp = handle_request(req);
        let v: serde_json::Value = serde_json::from_slice(&resp).unwrap();
        assert_eq!(v["type"], "error");
        assert_eq!(v["error"]["code"], "MALFORMED_REQUEST");
    }

    #[test]
    fn handle_parse_v2_lockfile_error_code() {
        let body = br#"{"lockfileVersion":2,"packages":{}}"#;
        let b64 = BASE64_STANDARD.encode(body);
        let req = serde_json::json!({
            "id": "parse-v2",
            "type": "parse_npm_lockfile",
            "payload": { "content": b64 }
        });
        let resp_bytes = handle_request(serde_json::to_vec(&req).unwrap().as_slice());
        let v: serde_json::Value = serde_json::from_slice(&resp_bytes).unwrap();
        assert_eq!(v["error"]["code"], "UNSUPPORTED_LOCKFILE_VERSION");
    }
}
