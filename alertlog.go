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
  ignore int
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
    ignore := 0
    for _ , e := range config.Cfgs[conf].Alertlog[0].Ignoreora {
      if e == ora {
        ignore = 1
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
  layout  := "Mon Jan 02 15:04:05 2006"
  loc     := time.Now().Location()
  re      := regexp.MustCompile(`ORA-[0-9]+`)

  for conf, _ := range config.Cfgs {
    var lastTime time.Time
    Errors = nil

    file, err := os.Open(config.Cfgs[conf].Alertlog[0].File)
    if err != nil {
      log.Infoln(err)
    } else{
      file.Close()

      scanner := bufio.NewScanner(file)
      for scanner.Scan() {
        t, err := time.ParseInLocation(layout, scanner.Text(),loc)
        if err == nil {
          lastTime = t
        } else {
          if int(time.Now().Sub(lastTime).Seconds()) < config.Cfgs[conf].Alertlog[0].Scantime {
            if re.MatchString(scanner.Text()) {
              ora := re.FindString(scanner.Text())
              addError(conf,ora, scanner.Text())
            }
          }
        }
      }
      for i, _ := range Errors {
        e.oraerror.WithLabelValues(config.Cfgs[conf].Database,
                                   config.Cfgs[conf].Instance,
                                   Errors[i].ora,
                                   Errors[i].text).Set(float64(Errors[i].count))
      }
    }
  }
}
