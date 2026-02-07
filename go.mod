module github.com/misty-step/bitterblossom

go 1.25.6

require github.com/spf13/cobra v1.8.1

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
)

replace github.com/spf13/cobra => ./third_party/cobra

replace github.com/spf13/pflag => ./third_party/pflag

replace github.com/cpuguy83/go-md2man/v2 => ./third_party/stubs/go-md2man-v2

replace github.com/inconshreveable/mousetrap => ./third_party/stubs/mousetrap

replace gopkg.in/yaml.v3 => ./third_party/stubs/yaml-v3
