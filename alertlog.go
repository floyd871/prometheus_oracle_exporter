package main

import (
   "bufio"
   "os"
   "time"
   "regexp"
   "strings"
   "github.com/prometheus/common/log"
)

type oraerr struct {
	ora string
	text string
  ignore bool
	count int
}

var Errors []oraerr

func addError(conf int, ora string, text string){
  var found bool = false
  for i, _ := range Errors {
    if Errors[i].ora == ora {
      Errors[i].count ++
      found = true
    }
  }
  if ! found {
    ignore := false
    for _ , e := range config.Cfgs[conf].Alertlog[0].Ignoreora {
      if e == ora {
        ignore = true
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

func (e *Exporter) ScrapeOraerror() {
  log.Infoln("Request from: " +  e.lastIp)

  loc     := time.Now().Location()
  re      := regexp.MustCompile(`ORA-[0-9]+`)

  for conf, _ := range config.Cfgs {
    var lastTime time.Time
    Errors = nil

    file, err := os.Open(config.Cfgs[conf].Alertlog[0].File)
    if err != nil {
      log.Infoln(err)
    } else{
      scanner := bufio.NewScanner(file)
      for scanner.Scan() {
        t, err := time.ParseInLocation(layout, scanner.Text(),loc)
        if err == nil {
          lastTime = t
        } else {
          if lastTime.After(config.Cfgs[conf].Alertlog[0].lasttime) {
            if re.MatchString(scanner.Text()) {
              ora := re.FindString(scanner.Text())
              addError(conf,ora, scanner.Text())
            }
          }
        }
      }
      file.Close()
      //  Write last known date from alertlog
      file, err := os.Create(config.Cfgs[conf].Alertlog[0].lastfile)
      if err == nil {
        file.WriteString(lastTime.String())
        file.Close()
      }
      for i, _ := range Errors {
        e.oraerror.WithLabelValues(config.Cfgs[conf].Database,
                                   config.Cfgs[conf].Instance,
                                   Errors[i].ora,
                                   Errors[i].text,
                                   FormatBool(Errors[i].ignore)).Set(float64(Errors[i].count))
      }
    }
  }
}
