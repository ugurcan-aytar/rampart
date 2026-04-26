//! npm package-lock.json (lockfileVersion: 3) parser.
//!
//! Byte-for-byte compatible with `engine/sbom/npm.Parser` in Go — field
//! names, ordering, scope semantics, and the PURL canonicalisation for
//! scoped packages all match. See `engine/sbom/npm/parity_test.go`.

use std::collections::BTreeMap;

use serde::Deserialize;

use super::{PackageVersion, ParseError, ParsedSbom};

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

/// Parse a `package-lock.json` body into a [`ParsedSbom`].
pub fn parse(content: &[u8]) -> Result<ParsedSbom, ParseError> {
    let lf: Lockfile = serde_json::from_slice(content)?;
    if lf.lockfile_version != 3 {
        return Err(ParseError::UnsupportedVersion(format!(
            "got {}, expected 3",
            lf.lockfile_version
        )));
    }
    let packages_map = lf
        .packages
        .ok_or_else(|| ParseError::Empty("`packages` map is absent".to_string()))?;

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
        assert!(matches!(err, ParseError::UnsupportedVersion(_)));
    }

    #[test]
    fn parse_rejects_missing_packages_key() {
        let body = r#"{"name":"x","lockfileVersion":3}"#;
        let err = parse(body.as_bytes()).unwrap_err();
        assert!(matches!(err, ParseError::Empty(_)));
    }

    #[test]
    fn parse_rejects_malformed_json() {
        let err = parse(b"{broken").unwrap_err();
        assert!(matches!(err, ParseError::Malformed(_)));
    }
}
