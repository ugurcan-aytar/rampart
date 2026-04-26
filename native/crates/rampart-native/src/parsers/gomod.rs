//! Go modules parser. Reads `go.sum` (the authoritative version + hash
//! list) and `go.mod` (the module identity + replace directives) and
//! returns the flat dependency list as a [`ParsedSbom`].
//!
//! Byte-for-byte compatible with `engine/sbom/gomod.Parser` in Go.
//! See `engine/sbom/gomod/parity_test.go`.
//!
//! Algorithm:
//!   1. Parse `go.mod` for `replace OLD [VER] => NEW [VER]` directives.
//!      Local replaces (`=> ./local`) drop the original from the output.
//!      Remote replaces substitute the right-hand-side path + version.
//!   2. Parse `go.sum` line-by-line. Each line is:
//!      `<module-path> <version>[/go.mod] h1:<hash>=`
//!      The primary line (no `/go.mod` suffix) carries the module zip
//!      hash; the `/go.mod` line hashes the go.mod itself. We prefer
//!      the primary; modules that only have the `/go.mod` line still
//!      get one entry.
//!   3. Apply replaces, sort by (name, version), emit.

use std::collections::BTreeMap;

use super::{PackageVersion, ParseError, ParsedSbom};

/// Replace target — `Some(version)` for remote replace, `None` for
/// local file-system replace which means "drop the original entry".
#[derive(Debug, Clone)]
struct ReplaceTarget {
    new_path: String,
    new_version: Option<String>,
}

/// Parse go.sum + go.mod into a `ParsedSbom`. `gomod_content` may be
/// empty (e.g. a tarball that only ships go.sum) — in that case no
/// replace directives apply.
pub fn parse(gosum_content: &[u8], gomod_content: &[u8]) -> Result<ParsedSbom, ParseError> {
    let replaces = parse_gomod_replaces(gomod_content)?;
    let entries = parse_gosum(gosum_content)?;

    let mut deduped: BTreeMap<(String, String), String> = BTreeMap::new();
    let mut prefer_primary: BTreeMap<(String, String), bool> = BTreeMap::new();
    for entry in entries {
        let key = (entry.module.clone(), entry.version.clone());
        let already_primary = *prefer_primary.get(&key).unwrap_or(&false);
        if already_primary {
            continue;
        }
        deduped.insert(key.clone(), entry.hash);
        prefer_primary.insert(key, entry.is_primary);
    }

    let mut packages: Vec<PackageVersion> = Vec::with_capacity(deduped.len());
    for ((module, version), hash) in deduped {
        let (name, ver) = match replaces.get(&module) {
            Some(target) => match &target.new_version {
                Some(new_ver) => (target.new_path.clone(), new_ver.clone()),
                None => continue, // local replace drops the entry
            },
            None => (module, version),
        };
        packages.push(PackageVersion {
            ecosystem: "gomod".to_string(),
            name: name.clone(),
            version: ver.clone(),
            purl: canonical_purl(&name, &ver),
            scope: None,
            integrity: hash,
        });
    }

    packages.sort_by(|a, b| a.name.cmp(&b.name).then(a.version.cmp(&b.version)));

    Ok(ParsedSbom {
        ecosystem: "gomod".to_string(),
        packages,
        source_format: "go-sum-v1".to_string(),
        source_bytes: gosum_content.len() as i64,
    })
}

/// One physical line from go.sum.
#[derive(Debug)]
struct SumEntry {
    module: String,
    version: String,
    hash: String,
    is_primary: bool, // true → main module hash; false → /go.mod hash
}

fn parse_gosum(content: &[u8]) -> Result<Vec<SumEntry>, ParseError> {
    let text = std::str::from_utf8(content)
        .map_err(|e| ParseError::Malformed(format!("go.sum is not utf-8: {e}")))?;
    let mut out = Vec::new();
    for (lineno, raw) in text.lines().enumerate() {
        let line = raw.trim();
        if line.is_empty() {
            continue;
        }
        let parts: Vec<&str> = line.split_whitespace().collect();
        if parts.len() != 3 {
            return Err(ParseError::Malformed(format!(
                "go.sum line {}: expected 3 fields, got {}",
                lineno + 1,
                parts.len()
            )));
        }
        let module = parts[0].to_string();
        let raw_version = parts[1];
        let hash = parts[2].to_string();
        if !hash.starts_with("h1:") {
            return Err(ParseError::Malformed(format!(
                "go.sum line {}: hash field must start with h1:",
                lineno + 1
            )));
        }
        let (version, is_primary) = if let Some(stripped) = raw_version.strip_suffix("/go.mod") {
            (stripped.to_string(), false)
        } else {
            (raw_version.to_string(), true)
        };
        out.push(SumEntry {
            module,
            version,
            hash,
            is_primary,
        });
    }
    Ok(out)
}

fn parse_gomod_replaces(content: &[u8]) -> Result<BTreeMap<String, ReplaceTarget>, ParseError> {
    if content.is_empty() {
        return Ok(BTreeMap::new());
    }
    let text = std::str::from_utf8(content)
        .map_err(|e| ParseError::Malformed(format!("go.mod is not utf-8: {e}")))?;
    let mut out = BTreeMap::new();
    let mut in_block = false;
    for raw in text.lines() {
        let line = strip_comment(raw).trim();
        if line.is_empty() {
            continue;
        }
        if in_block {
            if line == ")" {
                in_block = false;
                continue;
            }
            if let Some((src, tgt)) = parse_replace_directive(line) {
                out.insert(src, tgt);
            }
            continue;
        }
        if line == "replace (" {
            in_block = true;
            continue;
        }
        if let Some(rest) = line.strip_prefix("replace ") {
            if let Some((src, tgt)) = parse_replace_directive(rest.trim()) {
                out.insert(src, tgt);
            }
        }
    }
    Ok(out)
}

fn strip_comment(line: &str) -> &str {
    match line.find("//") {
        Some(i) => &line[..i],
        None => line,
    }
}

/// Parse `OLD [OLDVER] => NEW [NEWVER]`. Returns Some only when the
/// directive is well-formed; malformed lines are silently skipped
/// (they would already have failed `go mod tidy`).
fn parse_replace_directive(line: &str) -> Option<(String, ReplaceTarget)> {
    let (lhs, rhs) = line.split_once("=>")?;
    let lhs_parts: Vec<&str> = lhs.split_whitespace().collect();
    let rhs_parts: Vec<&str> = rhs.split_whitespace().collect();
    if lhs_parts.is_empty() || rhs_parts.is_empty() {
        return None;
    }
    let src = lhs_parts[0].to_string();
    let new_path = rhs_parts[0].to_string();
    // Local file-system replace: `=> ./relative/path` (no version, path
    // starts with `.` or `/`). Treated as drop-source.
    let is_local =
        new_path.starts_with('.') || new_path.starts_with('/') || rhs_parts.len() == 1;
    let new_version = if is_local {
        None
    } else if rhs_parts.len() >= 2 {
        Some(rhs_parts[1].to_string())
    } else {
        None
    };
    Some((
        src,
        ReplaceTarget {
            new_path,
            new_version,
        },
    ))
}

/// purl per https://github.com/package-url/purl-spec/blob/master/PURL-TYPES.rst#golang
fn canonical_purl(name: &str, version: &str) -> String {
    format!("pkg:golang/{name}@{version}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_simple_two_modules() {
        let gosum = b"github.com/spf13/cobra v1.8.0 h1:hashA=\n\
                      github.com/spf13/cobra v1.8.0/go.mod h1:hashAmod=\n\
                      github.com/stretchr/testify v1.9.0 h1:hashB=\n\
                      github.com/stretchr/testify v1.9.0/go.mod h1:hashBmod=\n";
        let gomod = b"module example.com/x\n\ngo 1.21\n";
        let sbom = parse(gosum, gomod).unwrap();
        assert_eq!(sbom.ecosystem, "gomod");
        assert_eq!(sbom.packages.len(), 2);
        assert_eq!(sbom.packages[0].name, "github.com/spf13/cobra");
        assert_eq!(sbom.packages[0].version, "v1.8.0");
        assert_eq!(sbom.packages[0].integrity, "h1:hashA=");
        assert_eq!(
            sbom.packages[0].purl,
            "pkg:golang/github.com/spf13/cobra@v1.8.0"
        );
    }

    #[test]
    fn parse_pseudo_version_passthrough() {
        let gosum = b"github.com/foo/bar v0.0.0-20240115123456-abcdef123456 h1:p=\n\
                      github.com/foo/bar v0.0.0-20240115123456-abcdef123456/go.mod h1:pm=\n";
        let sbom = parse(gosum, b"").unwrap();
        assert_eq!(sbom.packages.len(), 1);
        assert_eq!(
            sbom.packages[0].version,
            "v0.0.0-20240115123456-abcdef123456"
        );
    }

    #[test]
    fn parse_remote_replace_substitutes_target() {
        let gosum = b"github.com/new/lib v2.0.0 h1:nh=\n\
                      github.com/new/lib v2.0.0/go.mod h1:nhm=\n";
        let gomod = b"module example.com/x\nreplace github.com/old/lib => github.com/new/lib v2.0.0\n";
        let sbom = parse(gosum, gomod).unwrap();
        // Only the new module exists in go.sum (Go's behavior); the
        // replace target stays as it was.
        assert_eq!(sbom.packages.len(), 1);
        assert_eq!(sbom.packages[0].name, "github.com/new/lib");
    }

    #[test]
    fn parse_local_replace_drops_source() {
        let gosum = b"github.com/old/lib v1.0.0 h1:o=\n\
                      github.com/old/lib v1.0.0/go.mod h1:om=\n";
        let gomod = b"module example.com/x\nreplace github.com/old/lib => ./local-fork\n";
        let sbom = parse(gosum, gomod).unwrap();
        assert!(sbom.packages.is_empty(), "local replace should drop source");
    }

    #[test]
    fn parse_rejects_malformed_line() {
        let gosum = b"github.com/foo/bar v1.0.0\n"; // missing hash
        let err = parse(gosum, b"").unwrap_err();
        assert!(matches!(err, ParseError::Malformed(_)));
    }

    #[test]
    fn parse_rejects_non_h1_hash() {
        let gosum = b"github.com/foo/bar v1.0.0 sha256:abc=\n";
        let err = parse(gosum, b"").unwrap_err();
        assert!(matches!(err, ParseError::Malformed(_)));
    }

    #[test]
    fn parse_replace_block_form() {
        let gomod = b"module example.com/x\n\nreplace (\n\tgithub.com/old/a => github.com/new/a v1.0.0\n\tgithub.com/old/b => ./local\n)\n";
        let map = parse_gomod_replaces(gomod).unwrap();
        assert_eq!(map.len(), 2);
        assert_eq!(
            map.get("github.com/old/a").unwrap().new_path,
            "github.com/new/a"
        );
        assert!(map.get("github.com/old/b").unwrap().new_version.is_none());
    }

    #[test]
    fn purl_format_is_purl_spec_golang() {
        assert_eq!(
            canonical_purl("github.com/spf13/cobra", "v1.8.0"),
            "pkg:golang/github.com/spf13/cobra@v1.8.0"
        );
    }
}
