package main

import (
   "bufio"
   "fmt"
   "os"
   "time"
   "regexp"
   "github.com/prometheus/common/log"
)

func processAlertlog(fn string) {
  var lastTime time.Time

  layout := "Mon Jan 02 15:04:05 2006"
  loc := time.Now().Location()

  file, err := os.Open(fn)
  if err != nil {
    log.Fatal(err)
  }
  defer file.Close()

  scanner := bufio.NewScanner(file)
  for scanner.Scan() {
      t, err := time.ParseInLocation(layout, scanner.Text(),loc)
      if err == nil {
        lastTime = t
      } else {
        match, _ := regexp.MatchString("^ORA-[0-9]+", scanner.Text())
        if match {
          fmt.Println(lastTime)
          fmt.Println(scanner.Text())
        }
      }
  }

  if err := scanner.Err(); err != nil {
      log.Fatal(err)
  }
}
