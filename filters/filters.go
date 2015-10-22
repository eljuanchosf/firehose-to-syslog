package filters

import (
	"io/ioutil"
	"strings"

	"gopkg.in/yaml.v2"
)

type CFEntity struct {
	CFFieldName string `yaml:"cf_field_name"`
	Filters     []CFFilter
}

type CFFilter struct {
	Name string
	Type string
}

type LogFilters struct {
	Orgs   CFEntity
	Spaces CFEntity
	Apps   CFEntity
}

func ParseFilters(filtersDefinition string) (map[string][]string, error) {
	parsedFilters := make(map[string][]string)
	for _, section := range strings.Split(filtersDefinition, "|") {
		if section != "" {
			filter := strings.Split(section, ":")
			filterType := strings.TrimSpace(filter[0])
			filterDef := strings.TrimSpace(filter[1])
			filterValues := []string{}
			for _, filterValue := range strings.Split(filterDef, ",") {
				filterValues = append(filterValues, strings.TrimSpace(filterValue))
			}
			parsedFilters["cf_"+filterType] = filterValues
		}
	}
	return parsedFilters, nil
}

func GetFilters(filename string) LogFilters {
	var config LogFilters
	source, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	err = yaml.Unmarshal(source, &config)
	if err != nil {
		panic(err)
	}
	return config
}
