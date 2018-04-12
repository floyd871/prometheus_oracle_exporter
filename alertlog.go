package main

import (
   "bufio"
   "os"
   "time"
   "regexp"
   "strings"
   "io/ioutil"
   "github.com/prometheus/common/log"
)

type oraerr struct {
  ora string
  text string
  ignore string
  count int
}

var Errors []oraerr

// Get individual ScrapeTime per Prometheus instance for alertlog
func (e *Exporter) GetLastScrapeTime(conf int) time.Time {
  file :=  pwd + "/prometheus_" + config.Cfgs[conf].Instance + "_" + cleanIp(e.lastIp) + ".dat"
  content, err := ioutil.ReadFile(file)
  if err == nil {
    t, _ := time.Parse("2006-01-02 15:04:05 -0700 MST",string(content))
    return t
  }
  return time.Now()
}

// Set individual ScrapeTime per Prometheus instance for alertlog
func (e *Exporter) SetLastScrapeTime(conf int,t time.Time) {
  file :=  pwd + "/prometheus_" + config.Cfgs[conf].Instance + "_" + cleanIp(e.lastIp) + ".dat"
  fh, err := os.Create(file)
  if err == nil {
   fh.WriteString(t.String())
   fh.Close()
  }
}


func addError(conf int, ora string, text string){
  var found bool = false
  for i, _ := range Errors {
    if Errors[i].ora == ora {
      Errors[i].count ++
      found = true
    }
  }
  if ! found {
    ignore := "0"
    for _ , e := range config.Cfgs[conf].Alertlog[0].Ignoreora {
      if e == ora {
        ignore = "1"
      }
    }
    i := strings.Index(text, " ")
    if i < 0{
      i = 0
    }
    ora := oraerr{ora: ora, text: text[i+1:], ignore: ignore, count: 1}
    Errors = append (Errors, ora)
  }
}

func (e *Exporter) ScrapeAlertlog() {
  loc     := time.Now().Location()
  re      := regexp.MustCompile(`ORA-[0-9]+`)

  for conf, _ := range config.Cfgs {
    var lastTime time.Time
    Errors = nil
    lastScrapeTime := e.GetLastScrapeTime(conf)

    file, err := os.Open(config.Cfgs[conf].Alertlog[0].File)
    if err != nil {
      log.Infoln(err)
    } else{
      scanner := bufio.NewScanner(file)
      for scanner.Scan() {
        t, err := time.ParseInLocation(oralayout, scanner.Text(),loc)
        if err == nil {
          lastTime = t
        } else {
          if lastTime.After(lastScrapeTime) {
            if re.MatchString(scanner.Text()) {
              ora := re.FindString(scanner.Text())
              addError(conf,ora, scanner.Text())
            }
          }
        }
      }
      file.Close()
      e.SetLastScrapeTime(conf,lastTime)
      for i, _ := range Errors {
        e.alertlog.WithLabelValues(config.Cfgs[conf].Database,
                                   config.Cfgs[conf].Instance,
                                   Errors[i].ora,
                                   Errors[i].text,
                                   Errors[i].ignore).Set(float64(Errors[i].count))
      }
    }
  }
}
