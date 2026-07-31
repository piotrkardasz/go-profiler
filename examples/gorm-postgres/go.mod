module github.com/piotrkardasz/go-profiler/examples/gorm-postgres

go 1.26.1

require (
	github.com/piotrkardasz/go-profiler v0.0.0
	github.com/piotrkardasz/go-profiler/collector/gorm v0.0.0
	gorm.io/driver/postgres v1.5.11
	gorm.io/gorm v1.25.12
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/pgx/v5 v5.5.5 // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	golang.org/x/crypto v0.17.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/text v0.14.0 // indirect
)

replace (
	github.com/piotrkardasz/go-profiler => ../..
	github.com/piotrkardasz/go-profiler/collector/gorm => ../../collector/gorm
)
