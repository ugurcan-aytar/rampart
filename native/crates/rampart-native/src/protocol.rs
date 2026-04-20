//! Binary wire protocol for the UDS bridge.
//!
//! Framing: outer 4-byte big-endian length prefix, then a body.
//! Body always starts with a single byte **opcode** and is followed
//! by opcode-specific bytes.
//!
//! Request opcodes (client → server):
//!   * `0x01` parse_npm_lockfile
//!     - 4-byte BE content_length + `content_length` raw lockfile bytes
//!     - 4-byte BE metadata_length + `metadata_length` raw bytes
//!       (reserved — Phase 1 callers send 0 and the server ignores any
//!       payload; Phase 2 may use this as a JSON options blob)
//!   * `0xFF` ping — body is just the opcode byte
//!
//! Response opcodes (server → client):
//!   * `0x02` parse_result — 4-byte BE sbom_length + ParsedSBOM JSON
//!   * `0x03` error — 4-byte BE code_length + code (ASCII), then
//!     4-byte BE message_length + message (UTF-8)
//!   * `0xFE` pong — body is just the opcode byte
//!
//! The request path is fully binary so large lockfile blobs ride the
//! wire without base64 expansion or JSON escape cost. The response
//! payload stays JSON — the SBOM is orders of magnitude smaller than
//! a large lockfile, and the JSON form keeps `strace` / `tcpdump`
//! output readable.

use thiserror::Error;

/// Maximum outer body length. Matches the Go client's `MaxFrameBytes`.
pub const MAX_FRAME_BYTES: u32 = 100 * 1024 * 1024;

pub const MSG_PARSE_REQUEST: u8 = 0x01;
pub const MSG_PARSE_RESULT: u8 = 0x02;
pub const MSG_ERROR: u8 = 0x03;
pub const MSG_PONG: u8 = 0xFE;
pub const MSG_PING: u8 = 0xFF;

/// Decoded request — what the server dispatches on after a successful
/// binary decode of the body.
#[derive(Debug, PartialEq, Eq)]
pub enum Request {
    Parse {
        content: Vec<u8>,
        /// Reserved extension point (Phase 2: parser options). Phase 1
        /// callers send an empty slice; the server ignores any payload
        /// here so a forward-compatible client never breaks the
        /// protocol handshake.
        reserved_metadata: Vec<u8>,
    },
    Ping,
}

#[derive(Debug, Error, PartialEq, Eq)]
pub enum DecodeError {
    #[error("empty body")]
    EmptyBody,
    #[error("truncated frame")]
    Truncated,
    #[error("unexpected trailing bytes")]
    TrailingBytes,
    #[error("unknown opcode {0:#x}")]
    UnknownOpcode(u8),
}

/// Decode a request body (everything after the outer length prefix).
pub fn decode_request_body(body: &[u8]) -> Result<Request, DecodeError> {
    if body.is_empty() {
        return Err(DecodeError::EmptyBody);
    }
    let opcode = body[0];
    let rest = &body[1..];
    match opcode {
        MSG_PARSE_REQUEST => {
            let (content, rest) = read_length_prefixed(rest)?;
            let (metadata, rest) = read_length_prefixed(rest)?;
            if !rest.is_empty() {
                return Err(DecodeError::TrailingBytes);
            }
            Ok(Request::Parse {
                content: content.to_vec(),
                reserved_metadata: metadata.to_vec(),
            })
        }
        MSG_PING => {
            if !rest.is_empty() {
                return Err(DecodeError::TrailingBytes);
            }
            Ok(Request::Ping)
        }
        other => Err(DecodeError::UnknownOpcode(other)),
    }
}

fn read_length_prefixed(rest: &[u8]) -> Result<(&[u8], &[u8]), DecodeError> {
    if rest.len() < 4 {
        return Err(DecodeError::Truncated);
    }
    let len = u32::from_be_bytes([rest[0], rest[1], rest[2], rest[3]]) as usize;
    let after_prefix = &rest[4..];
    if after_prefix.len() < len {
        return Err(DecodeError::Truncated);
    }
    Ok((&after_prefix[..len], &after_prefix[len..]))
}

/// Encode a full `parse_result` frame (outer length + body).
pub fn encode_parse_result(sbom_json: &[u8]) -> Vec<u8> {
    // body = opcode + 4-byte len + JSON
    let body_len = 1 + 4 + sbom_json.len();
    let mut buf = Vec::with_capacity(4 + body_len);
    buf.extend_from_slice(&(body_len as u32).to_be_bytes());
    buf.push(MSG_PARSE_RESULT);
    buf.extend_from_slice(&(sbom_json.len() as u32).to_be_bytes());
    buf.extend_from_slice(sbom_json);
    buf
}

/// Encode a full `error` frame (outer length + body).
pub fn encode_error(code: &str, message: &str) -> Vec<u8> {
    let code_b = code.as_bytes();
    let msg_b = message.as_bytes();
    let body_len = 1 + 4 + code_b.len() + 4 + msg_b.len();
    let mut buf = Vec::with_capacity(4 + body_len);
    buf.extend_from_slice(&(body_len as u32).to_be_bytes());
    buf.push(MSG_ERROR);
    buf.extend_from_slice(&(code_b.len() as u32).to_be_bytes());
    buf.extend_from_slice(code_b);
    buf.extend_from_slice(&(msg_b.len() as u32).to_be_bytes());
    buf.extend_from_slice(msg_b);
    buf
}

/// Encode a pong frame (outer length + body).
pub fn encode_pong() -> Vec<u8> {
    let mut buf = Vec::with_capacity(5);
    buf.extend_from_slice(&1u32.to_be_bytes());
    buf.push(MSG_PONG);
    buf
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn decode_parse_request_happy_path() {
        let content = b"lockfile bytes";
        let meta = b"{}";
        let mut body = Vec::new();
        body.push(MSG_PARSE_REQUEST);
        body.extend_from_slice(&(content.len() as u32).to_be_bytes());
        body.extend_from_slice(content);
        body.extend_from_slice(&(meta.len() as u32).to_be_bytes());
        body.extend_from_slice(meta);

        let req = decode_request_body(&body).unwrap();
        assert_eq!(
            req,
            Request::Parse {
                content: content.to_vec(),
                reserved_metadata: meta.to_vec(),
            }
        );
    }

    #[test]
    fn decode_parse_request_with_zero_metadata() {
        let content = b"x";
        let mut body = Vec::new();
        body.push(MSG_PARSE_REQUEST);
        body.extend_from_slice(&(content.len() as u32).to_be_bytes());
        body.extend_from_slice(content);
        body.extend_from_slice(&0u32.to_be_bytes());

        match decode_request_body(&body).unwrap() {
            Request::Parse {
                content: c,
                reserved_metadata,
            } => {
                assert_eq!(c, b"x");
                assert!(reserved_metadata.is_empty());
            }
            other => panic!("expected Parse, got {other:?}"),
        }
    }

    #[test]
    fn decode_ping() {
        let body = [MSG_PING];
        assert_eq!(decode_request_body(&body).unwrap(), Request::Ping);
    }

    #[test]
    fn decode_rejects_empty() {
        assert_eq!(
            decode_request_body(&[]).unwrap_err(),
            DecodeError::EmptyBody
        );
    }

    #[test]
    fn decode_rejects_unknown_opcode() {
        assert_eq!(
            decode_request_body(&[0x7F]).unwrap_err(),
            DecodeError::UnknownOpcode(0x7F)
        );
    }

    #[test]
    fn decode_rejects_truncated_content_length() {
        let body = [MSG_PARSE_REQUEST, 0x00, 0x00]; // 3 bytes where 4 needed
        assert_eq!(
            decode_request_body(&body).unwrap_err(),
            DecodeError::Truncated
        );
    }

    #[test]
    fn decode_rejects_content_overrun() {
        // Length says 10 but only 3 bytes follow.
        let mut body = Vec::new();
        body.push(MSG_PARSE_REQUEST);
        body.extend_from_slice(&10u32.to_be_bytes());
        body.extend_from_slice(&[1, 2, 3]);
        assert_eq!(
            decode_request_body(&body).unwrap_err(),
            DecodeError::Truncated
        );
    }

    #[test]
    fn decode_rejects_trailing_bytes_on_parse() {
        // Valid content + metadata, plus junk after.
        let mut body = Vec::new();
        body.push(MSG_PARSE_REQUEST);
        body.extend_from_slice(&0u32.to_be_bytes()); // content_len = 0
        body.extend_from_slice(&0u32.to_be_bytes()); // metadata_len = 0
        body.push(0xAA); // trailing junk
        assert_eq!(
            decode_request_body(&body).unwrap_err(),
            DecodeError::TrailingBytes
        );
    }

    #[test]
    fn decode_rejects_trailing_bytes_on_ping() {
        let body = [MSG_PING, 0xAA];
        assert_eq!(
            decode_request_body(&body).unwrap_err(),
            DecodeError::TrailingBytes
        );
    }

    #[test]
    fn encode_parse_result_layout() {
        let payload = br#"{"Ecosystem":"npm"}"#;
        let frame = encode_parse_result(payload);
        // outer_len == body_len == 1 + 4 + payload.len
        let outer_len = u32::from_be_bytes([frame[0], frame[1], frame[2], frame[3]]) as usize;
        assert_eq!(outer_len, 1 + 4 + payload.len());
        assert_eq!(frame[4], MSG_PARSE_RESULT);
        let sbom_len = u32::from_be_bytes([frame[5], frame[6], frame[7], frame[8]]) as usize;
        assert_eq!(sbom_len, payload.len());
        assert_eq!(&frame[9..], payload);
    }

    #[test]
    fn encode_error_layout() {
        let frame = encode_error("MALFORMED_LOCKFILE", "bad json");
        let outer_len = u32::from_be_bytes([frame[0], frame[1], frame[2], frame[3]]) as usize;
        assert_eq!(frame.len(), 4 + outer_len);
        assert_eq!(frame[4], MSG_ERROR);
        let code_len = u32::from_be_bytes([frame[5], frame[6], frame[7], frame[8]]) as usize;
        assert_eq!(code_len, "MALFORMED_LOCKFILE".len());
        let code_end = 9 + code_len;
        assert_eq!(&frame[9..code_end], b"MALFORMED_LOCKFILE");
        let msg_len = u32::from_be_bytes([
            frame[code_end],
            frame[code_end + 1],
            frame[code_end + 2],
            frame[code_end + 3],
        ]) as usize;
        assert_eq!(msg_len, "bad json".len());
        assert_eq!(&frame[code_end + 4..], b"bad json");
    }

    #[test]
    fn encode_pong_layout() {
        let frame = encode_pong();
        assert_eq!(frame, vec![0, 0, 0, 1, MSG_PONG]);
    }
}
