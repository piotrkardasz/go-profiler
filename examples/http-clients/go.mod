module github.com/piotrkardasz/go-profiler/examples/http-clients

go 1.26.1

require (
	github.com/piotrkardasz/go-profiler v0.0.0
	github.com/piotrkardasz/go-profiler/collector/http v0.0.0
)

replace (
	github.com/piotrkardasz/go-profiler => ../..
	github.com/piotrkardasz/go-profiler/collector/http => ../../collector/http
)
