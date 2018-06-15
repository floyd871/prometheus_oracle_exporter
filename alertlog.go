package main

import (
   "bufio"
   "os"
   "time"
   "regexp"
   "strings"
   "strconv"
   "github.com/prometheus/common/log"
)


type Client struct {
  Ip string          `yaml:"ip"`
  Date string        `yaml:"date"`
}

type Lastlog struct {
  Instance string    `yaml:"instance"`
  Clients []Client   `yaml:"clients"`
}

type Lastlogs struct {
  Cfgs []Lastlog     `yaml:"lastlog"`
}

type oraerr struct {
  ora string
  text string
  ignore string
  count int
}

var (
  Errors      []oraerr
  oralayout   = "Mon Jan 02 15:04:05 2006"
  lastlog     Lastlogs
)


// Get individual ScrapeTime per Prometheus instance for alertlog
func (e *Exporter) GetLastScrapeTime(conf int) time.Time {
  for i, _ := range lastlog.Cfgs {
    if lastlog.Cfgs[i].Instance == config.Cfgs[conf].Instance {
      for n, _ := range lastlog.Cfgs[i].Clients {
        if lastlog.Cfgs[i].Clients[n].Ip == e.lastIp {
          t, _ := time.Parse("2006-01-02 15:04:05 -0700 MST",string(lastlog.Cfgs[i].Clients[n].Date))
          return t
        }
      }
    }
  }
  return time.Now()
}

// Set individual ScrapeTime per Prometheus instance for alertlog
func (e *Exporter) SetLastScrapeTime(conf int,t time.Time) {
  var indInst int = -1
  var indIp int = -1
  for i, _ := range lastlog.Cfgs {
    if lastlog.Cfgs[i].Instance == config.Cfgs[conf].Instance {
      indInst = i
      for n, _ := range lastlog.Cfgs[i].Clients {
        if lastlog.Cfgs[i].Clients[n].Ip == e.lastIp {
          indIp = n
        }
      }
    }
  }
  if indInst == -1 {
    cln := Client{Ip: e.lastIp, Date: t.String()}
    lastlog.Cfgs = append(lastlog.Cfgs, Lastlog{Instance: config.Cfgs[conf].Instance,
                                                Clients:  []Client{ cln } } )
  }else{
    if indIp == -1 {
      cln := Client{Ip: e.lastIp, Date: t.String()}
      lastlog.Cfgs[indInst].Clients = append(lastlog.Cfgs[indInst].Clients, cln)
    }else{
      lastlog.Cfgs[indInst].Clients[indIp].Date = t.String()
    }
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
      if e == ora {; ignore = "1"; }
    }
    is := strings.Index(text, " ")
    ip := strings.Index(text, ". ")
    if is < 0 {; is = 0; }
    if ip < 0 {; ip = len(text); }
    ora := oraerr{ora: ora, text: text[is+1:ip], ignore: ignore, count: 1}
    Errors = append (Errors, ora)
  }
}

func (e *Exporter) ScrapeAlertlog() {
  loc     := time.Now().Location()
  re      := regexp.MustCompile(`O(RA|GG)-[0-9]+`)

  ReadAccess()
  for conf, _ := range config.Cfgs {
    var lastTime time.Time
    Errors = nil
    lastScrapeTime := e.GetLastScrapeTime(conf).Add(time.Second)

    info, _ := os.Stat(config.Cfgs[conf].Alertlog[0].File)
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
        WriteLog(config.Cfgs[conf].Instance +
                 "(" + Errors[i].ignore + "/" + strconv.Itoa(Errors[i].count) + "): " +
                 Errors[i].ora + " - " + Errors[i].text)
      }
      e.alertdate.WithLabelValues(config.Cfgs[conf].Database,
                                  config.Cfgs[conf].Instance).Set(float64(info.ModTime().Unix()))
    }
  }
  WriteAccess()
}
