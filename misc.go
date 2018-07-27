package main

import (
	"database/sql"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-oci8"
	"github.com/prometheus/common/log"
	"gopkg.in/yaml.v2"
)

type Alert struct {
	File      string   `yaml:"file"`
	Ignoreora []string `yaml:"ignoreora"`
}

type Query struct {
	Sql  string `yaml:"sql"`
	Name string `yaml:"name"`
}

type Config struct {
	Connection string  `yaml:"connection"`
	Database   string  `yaml:"database"`
	Instance   string  `yaml:"instance"`
	Alertlog   []Alert `yaml:"alertlog"`
	Queries    []Query `yaml:"queries"`
	db         *sql.DB
}

type Configs struct {
	Cfgs []Config `yaml:"connections"`
}

var (
	config Configs
	pwd    string
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

func cleanIp(s string) string {
	s = strings.Replace(s, ":", "", -1)  // Remove spaces
	s = strings.Replace(s, ".", "_", -1) // Remove open parenthesis
	return s
}

func loadConfig() bool {
	path, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	pwd = path
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

func ReadAccess() {
	var file = pwd + "/" + *accessFile
	content, err := ioutil.ReadFile(file)
	if err == nil {
		err := yaml.Unmarshal(content, &lastlog)
		if err != nil {
			log.Fatalf("error1: %v", err)
		}
	}
}

func WriteAccess() {
	content, _ := yaml.Marshal(&lastlog)
	ioutil.WriteFile(pwd+"/"+*accessFile, content, 0644)
}

func WriteLog(message string) {
	fh, err := os.OpenFile(pwd+"/"+*logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		fh.Seek(0, 2)
		fh.WriteString(time.Now().Format("2006-01-02 15:04:05") + " " + message + "\n")
		fh.Close()
	}
}
