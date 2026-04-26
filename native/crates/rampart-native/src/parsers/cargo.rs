//! Cargo.lock parser. Cargo.lock is TOML; the relevant shape is the
//! `[[package]]` array. Workspace-local packages (no `source` field)
//! are skipped — they are the project itself, not pulled deps. Git
//! sources are tagged with `scope = ["git"]`; registry sources keep
//! `scope = None`.
//!
//! Byte-for-byte compatible with `engine/sbom/cargo.Parser` in Go.

use serde::Deserialize;

use super::{PackageVersion, ParseError, ParsedSbom};

#[derive(Debug, Deserialize)]
struct Lockfile {
    #[serde(default, rename = "package")]
    packages: Vec<LockPackage>,
}

#[derive(Debug, Deserialize)]
struct LockPackage {
    name: String,
    version: String,
    #[serde(default)]
    source: Option<String>,
    #[serde(default)]
    checksum: Option<String>,
}

const REGISTRY_PREFIX: &str = "registry+";
const GIT_PREFIX: &str = "git+";

pub fn parse(content: &[u8]) -> Result<ParsedSbom, ParseError> {
    let text = std::str::from_utf8(content)
        .map_err(|e| ParseError::Malformed(format!("Cargo.lock is not utf-8: {e}")))?;
    let lf: Lockfile = toml::from_str(text)?;
    if lf.packages.is_empty() {
        return Err(ParseError::Empty(
            "Cargo.lock has no [[package]] entries".to_string(),
        ));
    }

    let mut packages = Vec::with_capacity(lf.packages.len());
    for pkg in lf.packages {
        let Some(source) = &pkg.source else {
            // Workspace-local member; skip — same reasoning as npm
            // skipping the root manifest entry.
            continue;
        };
        let scope = if source.starts_with(GIT_PREFIX) {
            Some(vec!["git".to_string()])
        } else if source.starts_with(REGISTRY_PREFIX) {
            None
        } else {
            // Unknown source kind: still include it but flag the source
            // verbatim so an operator can audit it. Keep parity with Go.
            Some(vec![format!("source:{source}")])
        };
        let integrity = pkg.checksum.unwrap_or_default();
        let purl = canonical_purl(&pkg.name, &pkg.version);
        packages.push(PackageVersion {
            ecosystem: "cargo".to_string(),
            name: pkg.name,
            version: pkg.version,
            purl,
            scope,
            integrity,
        });
    }

    packages.sort_by(|a, b| a.name.cmp(&b.name).then(a.version.cmp(&b.version)));

    Ok(ParsedSbom {
        ecosystem: "cargo".to_string(),
        packages,
        source_format: "cargo-lock-v3".to_string(),
        source_bytes: content.len() as i64,
    })
}

/// purl per https://github.com/package-url/purl-spec/blob/master/PURL-TYPES.rst#cargo
fn canonical_purl(name: &str, version: &str) -> String {
    format!("pkg:cargo/{name}@{version}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_simple_registry_packages() {
        let content = br#"
version = 4

[[package]]
name = "serde"
version = "1.0.215"
source = "registry+https://github.com/rust-lang/crates.io-index"
checksum = "abc123"

[[package]]
name = "thiserror"
version = "2.0.0"
source = "registry+https://github.com/rust-lang/crates.io-index"
checksum = "def456"
"#;
        let sbom = parse(content).unwrap();
        assert_eq!(sbom.ecosystem, "cargo");
        assert_eq!(sbom.packages.len(), 2);
        assert_eq!(sbom.packages[0].name, "serde");
        assert_eq!(sbom.packages[0].version, "1.0.215");
        assert_eq!(sbom.packages[0].integrity, "abc123");
        assert!(sbom.packages[0].scope.is_none());
        assert_eq!(sbom.packages[0].purl, "pkg:cargo/serde@1.0.215");
    }

    #[test]
    fn parse_skips_workspace_member() {
        let content = br#"
version = 4

[[package]]
name = "rampart-native"
version = "0.1.0"

[[package]]
name = "serde"
version = "1.0.0"
source = "registry+https://github.com/rust-lang/crates.io-index"
checksum = "x"
"#;
        let sbom = parse(content).unwrap();
        assert_eq!(sbom.packages.len(), 1, "workspace member must be skipped");
        assert_eq!(sbom.packages[0].name, "serde");
    }

    #[test]
    fn parse_git_source_tagged() {
        let content = br#"
[[package]]
name = "exotic"
version = "0.1.0"
source = "git+https://github.com/rust-lang/exotic?branch=main#abcdef0123"
"#;
        let sbom = parse(content).unwrap();
        assert_eq!(sbom.packages.len(), 1);
        assert_eq!(sbom.packages[0].scope, Some(vec!["git".to_string()]));
        assert_eq!(sbom.packages[0].integrity, "");
    }

    #[test]
    fn parse_rejects_malformed_toml() {
        let err = parse(b"[[package]\nname = ").unwrap_err();
        assert!(matches!(err, ParseError::Malformed(_)));
    }

    #[test]
    fn parse_rejects_empty_packages() {
        let err = parse(b"version = 4\n").unwrap_err();
        assert!(matches!(err, ParseError::Empty(_)));
    }

    #[test]
    fn purl_format_is_purl_spec_cargo() {
        assert_eq!(canonical_purl("serde", "1.0.215"), "pkg:cargo/serde@1.0.215");
    }
}
