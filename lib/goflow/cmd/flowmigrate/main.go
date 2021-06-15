package main

// go install github.com/nyaruka/goflow/cmd/flowmigrate
// cat legacy_flow.json | flowmigrate
// cat legacy_export.json | jq '.flows[0]' | flowmigrate

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/nyaruka/goflow/flows/definition"
	"github.com/nyaruka/goflow/flows/definition/migrations"
	"github.com/nyaruka/goflow/utils/jsonx"

	"github.com/Masterminds/semver"
)

func main() {
	var toVersion, baseMediaURL string
	var pretty bool

	flags := flag.NewFlagSet("", flag.ExitOnError)
	flags.StringVar(&toVersion, "to", definition.CurrentSpecVersion.String(), "Target flow spec version")
	flags.StringVar(&baseMediaURL, "base-media-url", "", "Base URL for media files")
	flags.BoolVar(&pretty, "pretty", false, "Pretty format output")
	flags.Parse(os.Args[1:])

	reader := bufio.NewReader(os.Stdin)

	output, err := Migrate(reader, semver.MustParse(toVersion), baseMediaURL, pretty)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(string(output))
	}
}

// Migrate reads a flow definition as JSON and migrates it
func Migrate(reader io.Reader, toVersion *semver.Version, baseMediaURL string, pretty bool) ([]byte, error) {
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var migConfig *migrations.Config
	if baseMediaURL != "" {
		migConfig = &migrations.Config{BaseMediaURL: baseMediaURL}
	}

	migrated, err := migrations.MigrateToVersion(data, toVersion, migConfig)

	// if we've migrated to the engine version, validate the flow can be read by the engine
	if toVersion == nil || toVersion.Equal(definition.CurrentSpecVersion) {
		_, err = definition.ReadFlow(migrated, nil)
		if err != nil {
			return nil, err
		}
	}

	if pretty {
		return jsonx.MarshalPretty(json.RawMessage(migrated))
	}

	return migrated, nil
}
