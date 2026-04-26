//! rampart-native — multi-ecosystem lockfile parser and UDS IPC server.
//!
//! Public surface: ecosystem parsers in [`parsers`] (npm, gomod, cargo)
//! and the binary IPC server in [`ipc::serve`]. The `rampart-native-cli`
//! binary listens on a Unix Domain Socket and answers length-prefixed
//! binary requests.

pub mod ipc;
pub mod parsers;
pub mod protocol;

pub use parsers::{gomod, npm, PackageVersion, ParseError, ParsedSbom};
pub use protocol::{
    decode_request_body, encode_error, encode_parse_result, encode_pong, DecodeError, Request,
    MAX_FRAME_BYTES, MSG_ERROR, MSG_PARSE_GOMOD_REQUEST, MSG_PARSE_REQUEST, MSG_PARSE_RESULT,
    MSG_PING, MSG_PONG,
};
