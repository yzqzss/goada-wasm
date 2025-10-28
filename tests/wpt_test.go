package goadawasm_test

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"testing"

	goadawasm "github.com/yzqzss/goada-wasm"
)

//go:embed testdata/urltestdata.json
var urlTestData []byte

type args struct {
	Input    string
	Base     string
	Href     string
	Origin   string
	Protocol string
	Username string
	Password string
	Host     string
	Hostname string
	Port     string
	Pathname string
	Search   string
	Hash     string
	Failure  bool
}

func TestAdaWASM(t *testing.T) {
	var tests []args
	json.Unmarshal(urlTestData, &tests)
	var testNum int
	empty := args{}

	for _, tt := range tests {
		if tt == empty {
			continue
		}
		testNum++

		t.Run(strconv.Itoa(testNum), func(t *testing.T) {
			func(t *testing.T) {
				var got *goadawasm.Url
				var err error
				if tt.Base != "" {
					got, err = goadawasm.NewWithBase(tt.Input, tt.Base)
				} else {
					got, err = goadawasm.New(tt.Input)
				}
				if (err != nil) != tt.Failure {
					t.Errorf("NewWithBase(%v, %v) = '%v', error = '%v', wantErr %v", tt.Base, tt.Input, got, err, tt.Failure)
					return
				}
				if err != nil && tt.Failure {
					return
				}
				if err != nil {
					t.Logf("Base: '%v', Input: '%v', Expected: '%v', GOT: '%v'", tt.Base, tt.Input, tt.Href, got)
					t.Errorf("NewWithBase(%v, %v) error = '%v', wantErr %v", tt.Base, tt.Input, err, tt.Failure)
					return
				}

				if got.Href() != tt.Href {
					t.Logf("Base: '%v', Input: '%v'", tt.Base, tt.Input)
					t.Errorf("Href() got = '%v', want '%v'", got.Href(), tt.Href)
				}

				if got.Protocol() != tt.Protocol {
					t.Errorf("Scheme got = '%v', want '%v'", got.Protocol(), tt.Protocol)
				}

				if got.Username() != tt.Username {
					t.Errorf("User.Username() got = '%v', want '%v'", got.Username(), tt.Username)
				}

				if got.Password() != tt.Password {
					t.Errorf("User.Password() got = '%v', want '%v'", got.Password(), tt.Password)
				}

				if got.Host() != tt.Host {
					t.Errorf("Host got = '%v', want '%v'", got.Host(), tt.Host)
				}

				if got.Hostname() != tt.Hostname {
					t.Errorf("Hostname() got = '%v', want '%v'", got.Hostname(), tt.Hostname)
				}

				if got.Port() != tt.Port {
					t.Errorf("Port() got = '%v', want '%v'", got.Port(), tt.Port)
				}

				if got.Pathname() != tt.Pathname {
					t.Errorf("Path got = '%v', want '%v'", got.Pathname(), tt.Pathname)
				}

				if got.Search() != tt.Search {
					t.Errorf("RawQuery got = '%v', want '%v'", got.Search(), tt.Search)
				}

				if got.Hash() != tt.Hash {
					t.Errorf("Fragment got = '%v', want '%v'", got.Hash(), tt.Hash)
				}

				reparsed, err := goadawasm.New(got.Href())
				if err != nil {
					t.Errorf("Parse() error = '%v'", err)
					return
				}
				if got.Href() != reparsed.Href() {
					t.Errorf("Reparsing expected same result got = '%v', want '%v'", reparsed.Href(), got.Href())
				}
			}(t)
		})
	}
}
