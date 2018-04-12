package main

import (
		"strings"
    "io/ioutil"
    "gopkg.in/yaml.v2"
    "github.com/prometheus/common/log"
		"os"
		"time"
)

func FormatBool(b bool) string {
    if b {
        return "1"
    }
    return "0"
}

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
	pwd, err := os.Getwd()
  if err != nil {
		log.Fatalf("error: %v", err)
  }
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
		for conf, _ := range config.Cfgs {
			file :=  pwd + "/prometheus_" + config.Cfgs[conf].Instance + ".dat"
			config.Cfgs[conf].Alertlog[0].lastfile = file
			content, err := ioutil.ReadFile(file)
			if err == nil {
				t, _ := time.Parse(layout,string(content))
				config.Cfgs[conf].Alertlog[0].lasttime = t
			}else{
				config.Cfgs[conf].Alertlog[0].lasttime = time.Now()
			}
    }
		return true
  }
}
