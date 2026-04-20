module github.com/ugurcan-aytar/rampart/cli

go 1.23

require (
	github.com/stretchr/testify v1.10.0
	github.com/ugurcan-aytar/rampart/engine v0.0.0-00010101000000-000000000000
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/oklog/ulid/v2 v2.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/ugurcan-aytar/rampart/engine => ../engine
