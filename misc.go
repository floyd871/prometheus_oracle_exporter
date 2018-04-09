package main

import (
		"strings"
    "io/ioutil"
    "gopkg.in/yaml.v2"
    "github.com/prometheus/common/log"
)

// Oracle gives us some ugly names back. This function cleans things up for Prometheus.
func cleanName(s string) string {
	s = strings.Replace(s, " ", "_", -1) // Remove spaces
	s = strings.Replace(s, "(", "", -1)  // Remove open parenthesis
	s = strings.Replace(s, ")", "", -1)  // Remove close parenthesis
	s = strings.Replace(s, "/", "", -1)  // Remove forward slashes
	s = strings.ToLower(s)
	return s
}

func loadConfig() bool {
  content, err := ioutil.ReadFile(*configFile)
	if err != nil {
			log.Fatalf("error: %v", err)
			return false
	} else {
		err := yaml.Unmarshal(content, &config)
		if err != nil {
			log.Fatalf("error: %v", err)
			return false
	  }
		return true
  }
}
