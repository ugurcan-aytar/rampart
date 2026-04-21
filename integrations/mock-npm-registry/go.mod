// Mock npm registry for rampart demos. Pure stdlib — no external
// dependencies, intentionally small. Runs in the Adım 7.5 demo stack
// to stand in for registry.npmjs.org without hitting the real
// internet.
module github.com/ugurcan-aytar/rampart/integrations/mock-npm-registry

go 1.23
