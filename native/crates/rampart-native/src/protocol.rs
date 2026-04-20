//! Wire protocol types for the UDS bridge.
//!
//! Transport: length-prefixed JSON — 4-byte big-endian u32 length header
//! followed by that many bytes of JSON body. The client writes exactly
//! that, reads exactly that back. Keeps the on-wire framing trivial
//! enough to `tcpdump` / `strace` in plaintext when debugging.

use serde::{Deserialize, Serialize};

/// Incoming request from the Go client.
///
/// Design: a single `type` discriminator + an inner payload enum so we
/// can add new request kinds (`parse_pypi_lockfile`, `parse_cargo_lockfile`,
/// …) without breaking older clients. Phase 1 ships three types:
/// `parse_npm_lockfile`, `ping`, `shutdown`.
#[derive(Debug, Deserialize)]
pub struct Request {
    pub id: String,
    #[serde(rename = "type")]
    pub kind: String,
    #[serde(default)]
    pub payload: Option<RequestPayload>,
}

/// Payload variants for each request kind. A Phase 2 addition becomes
/// another variant and leaves older code compiling.
#[derive(Debug, Deserialize)]
#[serde(untagged)]
pub enum RequestPayload {
    ParseNpmLockfile(ParseNpmLockfilePayload),
    Empty {},
}

#[derive(Debug, Deserialize)]
pub struct ParseNpmLockfilePayload {
    /// Base64-encoded lockfile bytes. Binary-safe framing so a bare-JSON
    /// wire doesn't need to worry about escaping a giant blob. This is
    /// the only field — identity (ID, GeneratedAt, ComponentRef,
    /// CommitSHA) is the engine's responsibility; see ADR-0005.
    pub content: String,
}

/// Response envelope: either a `payload` object (success) or an `error`
/// object (failure). Exactly one of the two is populated.
#[derive(Debug, Serialize)]
pub struct Response {
    pub id: String,
    #[serde(rename = "type")]
    pub kind: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub payload: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<ResponseError>,
}

#[derive(Debug, Serialize)]
pub struct ResponseError {
    pub code: String,
    pub message: String,
}

#[derive(Debug, Serialize)]
pub struct ParseStats {
    pub parse_ms: u64,
    pub package_count: usize,
    pub bytes_read: usize,
}

/// Response body for a successful `parse_npm_lockfile`. The payload
/// carries the pure parse result — the Go engine wraps it into a full
/// SBOM (ID, GeneratedAt, ComponentRef, CommitSHA) on its side.
#[derive(Debug, Serialize)]
pub struct ParseResult {
    pub parsed_sbom: crate::parser::ParsedSbom,
    pub stats: ParseStats,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn request_ping_roundtrip() {
        let raw = r#"{"id":"r1","type":"ping"}"#;
        let req: Request = serde_json::from_str(raw).unwrap();
        assert_eq!(req.kind, "ping");
    }

    #[test]
    fn request_parse_npm_lockfile_roundtrip() {
        let raw = r#"{
            "id": "r2",
            "type": "parse_npm_lockfile",
            "payload": {
                "content": "eyJsb2NrZmlsZVZlcnNpb24iOjN9"
            }
        }"#;
        let req: Request = serde_json::from_str(raw).unwrap();
        assert_eq!(req.kind, "parse_npm_lockfile");
        match req.payload {
            Some(RequestPayload::ParseNpmLockfile(p)) => {
                assert!(!p.content.is_empty());
            }
            _ => panic!("expected ParseNpmLockfile payload"),
        }
    }

    #[test]
    fn response_success_shape() {
        let resp = Response {
            id: "r1".to_string(),
            kind: "pong".to_string(),
            payload: Some(serde_json::json!({"ok": true})),
            error: None,
        };
        let s = serde_json::to_string(&resp).unwrap();
        assert!(s.contains("\"type\":\"pong\""));
        assert!(!s.contains("\"error\""));
    }

    #[test]
    fn response_error_shape() {
        let resp = Response {
            id: "r1".to_string(),
            kind: "error".to_string(),
            payload: None,
            error: Some(ResponseError {
                code: "UNSUPPORTED_LOCKFILE_VERSION".to_string(),
                message: "got 2, expected 3".to_string(),
            }),
        };
        let s = serde_json::to_string(&resp).unwrap();
        assert!(s.contains("\"code\":\"UNSUPPORTED_LOCKFILE_VERSION\""));
        assert!(!s.contains("\"payload\""));
    }
}
