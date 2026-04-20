//! npm package-lock.json (lockfileVersion: 3) parser.
//!
//! Byte-for-byte compatible with `engine/sbom/npm.Parser` in Go — field
//! names, ordering, scope semantics, and the PURL canonicalisation for
//! scoped packages all match. The Go side's `parity_test.go` diffs the
//! two outputs after a `canonical_json` marshal on every fixture in
//! `engine/testdata/lockfiles/` (no normalisation shim — see ADR-0005).
//!
//! The parser is pure: it owns lockfile-bytes-to-packages logic and
//! nothing else. Identity (SBOM ID, GeneratedAt, ComponentRef,
//! CommitSHA) is stamped engine-side in `engine/internal/ingestion`.
//!
//! What this parser does NOT do:
//!   - v1 / v2 lockfiles: returns [`ParseError::UnsupportedVersion`]
//!   - dependency-graph edges: only a flat package list (SBOM shape)
//!   - IoC matching: Phase 3 work, separate layer

use std::collections::BTreeMap;
use std::string::ToString;

use serde::{Deserialize, Serialize};
use thiserror::Error;

/// Parser errors. Flavoured to match the Go side's sentinel errors
/// (ErrMalformedLockfile / ErrUnsupportedLockfileVersion / ErrEmptyLockfile);
/// the protocol layer maps each variant to an `error.code` for the wire.
#[derive(Debug, Error)]
pub enum ParseError {
    #[error("malformed lockfile: {0}")]
    Malformed(#[from] serde_json::Error),
    #[error("unsupported lockfile version: got {0}, expected 3")]
    UnsupportedVersion(i64),
    #[error("empty lockfile: `packages` map is absent")]
    Empty,
}

#[derive(Debug, Deserialize)]
struct Lockfile {
    #[serde(default, rename = "lockfileVersion")]
    lockfile_version: i64,
    packages: Option<BTreeMap<String, LockPackage>>,
}

#[derive(Debug, Deserialize)]
struct LockPackage {
    #[serde(default)]
    version: Option<String>,
    #[serde(default)]
    integrity: Option<String>,
    #[serde(default)]
    dev: bool,
    #[serde(default)]
    optional: bool,
    #[serde(default)]
    peer: bool,
    #[serde(default)]
    link: bool,
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

/// Parse a `package-lock.json` body into a [`ParsedSbom`].
///
/// The Go parser at `engine/sbom/npm/parser.go` is the reference
/// implementation; every filtering / skipping rule here mirrors a
/// specific branch there:
///
///   - root entry (`""` key) → skipped
///   - `link: true` workspace symlinks → skipped
///   - paths without `node_modules/` in them (workspace source paths) → skipped
///   - missing `version` → skipped (parity: Go does the same)
///
/// Packages are sorted by (name, version) before return — matches Go's
/// `sort.Slice` call; important for the byte-identical parity test.
pub fn parse(content: &[u8]) -> Result<ParsedSbom, ParseError> {
    let lf: Lockfile = serde_json::from_slice(content)?;
    if lf.lockfile_version != 3 {
        return Err(ParseError::UnsupportedVersion(lf.lockfile_version));
    }
    let packages_map = lf.packages.ok_or(ParseError::Empty)?;

    let mut packages = Vec::with_capacity(packages_map.len());
    for (path, pkg) in &packages_map {
        if should_skip(path, pkg) {
            continue;
        }
        let Some(version) = &pkg.version else {
            continue;
        };
        let name = extract_name(path);
        let scope = build_scope(pkg);
        let integrity = pkg.integrity.clone().unwrap_or_default();

        packages.push(PackageVersion {
            ecosystem: "npm".to_string(),
            name: name.clone(),
            version: version.clone(),
            purl: canonical_purl(&name, version),
            scope,
            integrity,
        });
    }

    // Deterministic ordering so parity with Go holds across re-runs and
    // HashMap iteration order variations. Go parser uses sort.Slice on
    // (Name, Version).
    packages.sort_by(|a, b| a.name.cmp(&b.name).then(a.version.cmp(&b.version)));

    Ok(ParsedSbom {
        ecosystem: "npm".to_string(),
        packages,
        source_format: "npm-package-lock-v3".to_string(),
        source_bytes: content.len() as i64,
    })
}

fn should_skip(path: &str, pkg: &LockPackage) -> bool {
    if path.is_empty() || pkg.link {
        return true;
    }
    if !path.contains("node_modules/") {
        return true;
    }
    false
}

/// Extracts a bare package name from a lockfile path.
///
/// - `node_modules/axios` → `axios`
/// - `node_modules/@types/node` → `@types/node`
/// - `node_modules/outer/node_modules/nested` → `nested` (deepest wins)
/// - `node_modules/outer/node_modules/@scope/nested` → `@scope/nested`
fn extract_name(path: &str) -> String {
    const MARKER: &str = "node_modules/";
    match path.rfind(MARKER) {
        Some(idx) => path[idx + MARKER.len()..].to_string(),
        None => path.to_string(),
    }
}

fn build_scope(pkg: &LockPackage) -> Option<Vec<String>> {
    let mut scope = Vec::with_capacity(3);
    if pkg.dev {
        scope.push("dev".to_string());
    }
    if pkg.optional {
        scope.push("optional".to_string());
    }
    if pkg.peer {
        scope.push("peer".to_string());
    }
    if scope.is_empty() {
        None
    } else {
        Some(scope)
    }
}

/// Canonical purl per the purl spec. Scoped npm names URL-encode their
/// leading `@` (spec ref: https://github.com/package-url/purl-spec/blob/master/PURL-TYPES.rst#npm).
fn canonical_purl(name: &str, version: &str) -> String {
    if let Some(rest) = name.strip_prefix('@') {
        if let Some(slash) = rest.find('/') {
            let (ns, pkg) = (&rest[..slash], &rest[slash + 1..]);
            return format!("pkg:npm/%40{ns}/{pkg}@{version}");
        }
    }
    format!("pkg:npm/{name}@{version}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn extract_name_cases() {
        assert_eq!(extract_name("node_modules/axios"), "axios");
        assert_eq!(extract_name("node_modules/@types/node"), "@types/node");
        assert_eq!(
            extract_name("node_modules/outer/node_modules/nested"),
            "nested"
        );
        assert_eq!(
            extract_name("node_modules/outer/node_modules/@scope/nested"),
            "@scope/nested"
        );
    }

    #[test]
    fn canonical_purl_plain() {
        assert_eq!(canonical_purl("axios", "1.11.0"), "pkg:npm/axios@1.11.0");
    }

    #[test]
    fn canonical_purl_scoped() {
        assert_eq!(
            canonical_purl("@types/node", "22.0.0"),
            "pkg:npm/%40types/node@22.0.0"
        );
        assert_eq!(
            canonical_purl("@backstage/core-components", "0.15.0"),
            "pkg:npm/%40backstage/core-components@0.15.0"
        );
    }

    #[test]
    fn build_scope_combinations() {
        let none = LockPackage {
            version: None,
            integrity: None,
            dev: false,
            optional: false,
            peer: false,
            link: false,
        };
        assert_eq!(build_scope(&none), None);

        let dev = LockPackage {
            dev: true,
            ..none_mock()
        };
        assert_eq!(build_scope(&dev), Some(vec!["dev".to_string()]));

        let dev_peer = LockPackage {
            dev: true,
            peer: true,
            ..none_mock()
        };
        assert_eq!(
            build_scope(&dev_peer),
            Some(vec!["dev".to_string(), "peer".to_string()])
        );
    }

    fn none_mock() -> LockPackage {
        LockPackage {
            version: None,
            integrity: None,
            dev: false,
            optional: false,
            peer: false,
            link: false,
        }
    }

    #[test]
    fn parse_minimal_yields_empty_packages() {
        let body = r#"{"name":"x","version":"1","lockfileVersion":3,"packages":{"": {"name":"x","version":"1"}}}"#;
        let sbom = parse(body.as_bytes()).expect("should parse");
        assert!(sbom.packages.is_empty());
        assert_eq!(sbom.ecosystem, "npm");
        assert_eq!(sbom.source_format, "npm-package-lock-v3");
    }

    #[test]
    fn parse_rejects_v2() {
        let body = r#"{"lockfileVersion":2,"packages":{}}"#;
        let err = parse(body.as_bytes()).unwrap_err();
        assert!(matches!(err, ParseError::UnsupportedVersion(2)));
    }

    #[test]
    fn parse_rejects_missing_packages_key() {
        let body = r#"{"name":"x","lockfileVersion":3}"#;
        let err = parse(body.as_bytes()).unwrap_err();
        assert!(matches!(err, ParseError::Empty));
    }

    #[test]
    fn parse_rejects_malformed_json() {
        let err = parse(b"{broken").unwrap_err();
        assert!(matches!(err, ParseError::Malformed(_)));
    }
}
