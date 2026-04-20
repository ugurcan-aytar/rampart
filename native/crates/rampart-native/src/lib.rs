//! rampart-native — npm lockfile parser and UDS IPC server.
//!
//! The public surface of this crate is intentionally tiny: the server loop
//! in [`ipc::serve`] plus the parser entry point in [`parser::parse`].
//! `rampart-native-cli` turns the crate into a binary that listens on a
//! Unix Domain Socket and answers length-prefixed JSON requests.

pub mod ipc;
pub mod parser;
pub mod protocol;

pub use parser::{parse, PackageVersion, ParseError, ParsedSbom};
pub use protocol::{
    decode_request_body, encode_error, encode_parse_result, encode_pong, DecodeError, Request,
    MAX_FRAME_BYTES, MSG_ERROR, MSG_PARSE_REQUEST, MSG_PARSE_RESULT, MSG_PING, MSG_PONG,
};
