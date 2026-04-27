package maven_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/sbom/maven"
)

func TestParser_Pom_HappyPath(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lockfiles", "maven",
		"simple-pom", "pom.xml"))
	require.NoError(t, err)

	parsed, err := maven.NewParser().Parse(context.Background(), body, maven.FormatPom)
	require.NoError(t, err)
	require.Equal(t, "maven", parsed.Ecosystem)
	require.Equal(t, "maven-pom-v4", parsed.SourceFormat)
	require.Equal(t, int64(len(body)), parsed.SourceBytes)

	// 7 dependencies in the fixture; all kept (test scope is carried,
	// not dropped). Unresolved property kept verbatim with `${…}`.
	require.Len(t, parsed.Packages, 7)

	byName := map[string]string{}
	for _, p := range parsed.Packages {
		byName[p.Name] = p.Version
		require.Equal(t, "maven", p.Ecosystem)
	}
	// Property substitution resolved.
	require.Equal(t, "6.1.2", byName["org.springframework:spring-core"])
	require.Equal(t, "6.1.2", byName["org.springframework:spring-web"])
	require.Equal(t, "2.16.1", byName["com.fasterxml.jackson.core:jackson-databind"])
	// Literal version untouched.
	require.Equal(t, "2.0.11", byName["org.slf4j:slf4j-api"])
	require.Equal(t, "33.0.0-jre", byName["com.google.guava:guava"])
	// Test-scoped dependency kept (operator can post-filter).
	require.Equal(t, "5.10.1", byName["org.junit.jupiter:junit-jupiter"])
	// Unresolved property surfaces verbatim — operator follows the
	// `${unset.property}` thread rather than silently losing the dep.
	require.Equal(t, "${unset.property}", byName["com.example:unresolved-artifact"])
}

func TestParser_Pom_PURL(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lockfiles", "maven",
		"simple-pom", "pom.xml"))
	require.NoError(t, err)
	parsed, _ := maven.NewParser().Parse(context.Background(), body, maven.FormatPom)
	for _, p := range parsed.Packages {
		if p.Name == "org.springframework:spring-core" {
			require.Equal(t, "pkg:maven/org.springframework/spring-core@6.1.2", p.PURL)
		}
	}
}

func TestParser_Pom_TestScopeCarried(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lockfiles", "maven",
		"simple-pom", "pom.xml"))
	require.NoError(t, err)
	parsed, _ := maven.NewParser().Parse(context.Background(), body, maven.FormatPom)
	for _, p := range parsed.Packages {
		if p.Name == "org.junit.jupiter:junit-jupiter" {
			require.Equal(t, []string{"test"}, p.Scope)
		}
	}
}

func TestParser_GradleLock_HappyPath(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lockfiles", "maven",
		"gradle-lock", "gradle.lockfile"))
	require.NoError(t, err)

	parsed, err := maven.NewParser().Parse(context.Background(), body, maven.FormatGradle)
	require.NoError(t, err)
	require.Equal(t, "maven", parsed.Ecosystem)
	require.Equal(t, "gradle-lockfile-v1", parsed.SourceFormat)

	// 6 coordinate lines + 1 `empty=` sentinel that must be skipped.
	require.Len(t, parsed.Packages, 6)
	names := []string{}
	for _, p := range parsed.Packages {
		names = append(names, p.Name)
	}
	require.ElementsMatch(t, []string{
		"com.fasterxml.jackson.core:jackson-databind",
		"com.google.guava:guava",
		"io.netty:netty-all",
		"org.slf4j:slf4j-api",
		"org.springframework:spring-core",
		"org.springframework:spring-web",
	}, names)
}

func TestParser_GradleLock_PURL(t *testing.T) {
	body := []byte("io.netty:netty-all:4.1.106.Final=runtimeClasspath\n")
	parsed, err := maven.NewParser().Parse(context.Background(), body, maven.FormatGradle)
	require.NoError(t, err)
	require.Len(t, parsed.Packages, 1)
	require.Equal(t, "pkg:maven/io.netty/netty-all@4.1.106.Final", parsed.Packages[0].PURL)
}

func TestParser_UnknownFormat_Error(t *testing.T) {
	_, err := maven.NewParser().Parse(context.Background(), []byte("anything"), maven.Format("nonsense"))
	require.Error(t, err)
	require.True(t, errors.Is(err, maven.ErrMalformedLockfile))
}

func TestParser_MalformedXML_Error(t *testing.T) {
	body := []byte("<project><dependencies><broken")
	_, err := maven.NewParser().Parse(context.Background(), body, maven.FormatPom)
	require.Error(t, err)
	require.True(t, errors.Is(err, maven.ErrMalformedLockfile))
}
