module github.com/prejudice-studio/twilight

go 1.23.0

toolchain go1.23.2

require (
	github.com/jackc/pgx/v5 v5.6.0
	github.com/spf13/viper v1.21.0
	go.uber.org/zap v1.28.0
)

require (
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.17.0 // indirect
	golang.org/x/sync v0.15.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.28.0 // indirect
)

replace github.com/jackc/pgservicefile => github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761

replace github.com/jackc/puddle/v2 => github.com/jackc/puddle/v2 v2.2.2

replace golang.org/x/crypto => golang.org/x/crypto v0.39.0

replace github.com/stretchr/testify => github.com/stretchr/testify v1.10.0

replace golang.org/x/sync => golang.org/x/sync v0.7.0

replace golang.org/x/text => golang.org/x/text v0.26.0
