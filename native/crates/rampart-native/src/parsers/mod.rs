//! Per-ecosystem lockfile parsers + the shared `ParsedSbom` /
//! `PackageVersion` shape they all return.
//!
//! Each ecosystem module exposes a `parse(...) -> Result<ParsedSbom,
//! ParseError>` entry point. The Go side at `engine/sbom/<eco>/`
//! mirrors the same algorithm and the parity test in each ecosystem's
//! `parity_test.go` diffs the JSON output byte-for-byte.

use serde::{Deserialize, Serialize};
use thiserror::Error;

pub mod gomod;
pub mod npm;

/// Parser errors. The protocol layer maps each variant to an
/// `error.code` string on the wire (see `ipc::handle_*` dispatchers).
#[derive(Debug, Error)]
pub enum ParseError {
    #[error("malformed lockfile: {0}")]
    Malformed(String),
    #[error("unsupported lockfile version: {0}")]
    UnsupportedVersion(String),
    #[error("empty lockfile: {0}")]
    Empty(String),
}

impl From<serde_json::Error> for ParseError {
    fn from(e: serde_json::Error) -> Self {
        ParseError::Malformed(e.to_string())
    }
}

/// Pure parse result — no ID, no GeneratedAt, no ComponentRef, no
/// CommitSHA. The engine wraps this into a full `domain.SBOM` via
/// `engine/internal/ingestion.Ingest`. The `#[serde(rename = "…")]`
/// dance keeps field names identical to Go's default struct-field
/// serialisation so the parity test can diff byte-for-byte.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ParsedSbom {
    #[serde(rename = "Ecosystem")]
    pub ecosystem: String,
    #[serde(rename = "Packages")]
    pub packages: Vec<PackageVersion>,
    #[serde(rename = "SourceFormat")]
    pub source_format: String,
    #[serde(rename = "SourceBytes")]
    pub source_bytes: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct PackageVersion {
    #[serde(rename = "Ecosystem")]
    pub ecosystem: String,
    #[serde(rename = "Name")]
    pub name: String,
    #[serde(rename = "Version")]
    pub version: String,
    #[serde(rename = "PURL")]
    pub purl: String,
    // Option<Vec<String>> serialises as `null` when None — matches Go's
    // `json.Marshal` of a nil []string. Empty Vec would become `[]`,
    // which the Go side never emits for no-scope packages.
    #[serde(rename = "Scope")]
    pub scope: Option<Vec<String>>,
    #[serde(rename = "Integrity")]
    pub integrity: String,
}
