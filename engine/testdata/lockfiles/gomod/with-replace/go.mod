module example.com/with-replace

go 1.21

require (
	github.com/forked/lib v1.0.0
	github.com/replaced/dep v2.0.0
)

replace (
	github.com/replaced/dep => github.com/upstream/dep v3.5.0
	github.com/local/fork => ./vendor/local-fork
)
