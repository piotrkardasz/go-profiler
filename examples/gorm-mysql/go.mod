module github.com/piotrkardasz/go-profiler/examples/gorm-mysql

go 1.26.1

require (
	github.com/piotrkardasz/go-profiler v0.0.0
	github.com/piotrkardasz/go-profiler/collector/gorm v0.0.0
	gorm.io/driver/mysql v1.5.7
	gorm.io/gorm v1.25.12
)

require (
	github.com/go-sql-driver/mysql v1.7.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	golang.org/x/text v0.14.0 // indirect
)

replace (
	github.com/piotrkardasz/go-profiler => ../..
	github.com/piotrkardasz/go-profiler/collector/gorm => ../../collector/gorm
)
