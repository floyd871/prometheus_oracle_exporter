package main

import (
    "strings"
    "io/ioutil"
    "gopkg.in/yaml.v2"
    "github.com/prometheus/common/log"
    "os"
    "time"
    "path/filepath"
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
  s = strings.Replace(s, ":", "", -1) // Remove spaces
  s = strings.Replace(s, ".", "_", -1)  // Remove open parenthesis
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

func WriteLog(message string) {
  file :=  pwd + "/" + *logFile
  fh, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
  if err == nil {
   fh.WriteString(time.Now().Format("2006-01-02 15:04:05") + " " + message + "\n")
   fh.Close()
  }
}
