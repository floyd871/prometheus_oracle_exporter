package main

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/prometheus/common/log"
)

type Client struct {
	Ip   string `yaml:"ip"`
	Date string `yaml:"date"`
}

type Lastlog struct {
	Instance string   `yaml:"instance"`
	Clients  []Client `yaml:"clients"`
}

type Lastlogs struct {
	Cfgs []Lastlog `yaml:"lastlog"`
}

type oraerr struct {
	ora   string
	count int
}

var (
	Errors    []oraerr
	oralayout = "Mon Jan 02 15:04:05 2006"
	ogglayout = "2006-01-02 15:04:05"
	lastlog   Lastlogs
)

// Get individual ScrapeTime per Prometheus instance for alertlog
func (e *Exporter) GetLastScrapeTime(conf int) time.Time {
	for i, _ := range lastlog.Cfgs {
		if lastlog.Cfgs[i].Instance == config.Cfgs[conf].Instance {
			for n, _ := range lastlog.Cfgs[i].Clients {
				if lastlog.Cfgs[i].Clients[n].Ip == e.lastIp {
					t, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", string(lastlog.Cfgs[i].Clients[n].Date))
					return t
				}
			}
		}
	}
	return time.Now()
}

// Set individual ScrapeTime per Prometheus instance for alertlog
func (e *Exporter) SetLastScrapeTime(conf int, t time.Time) {
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
		cln := Client{Ip: e.lastIp, Date: t.Format("2006-01-02 15:04:05 -0700 MST")}
		lastlog.Cfgs = append(lastlog.Cfgs, Lastlog{Instance: config.Cfgs[conf].Instance,
			Clients: []Client{cln}})
	} else {
		if indIp == -1 {
			cln := Client{Ip: e.lastIp, Date: t.Format("2006-01-02 15:04:05 -0700 MST")}
			lastlog.Cfgs[indInst].Clients = append(lastlog.Cfgs[indInst].Clients, cln)
		} else {
			lastlog.Cfgs[indInst].Clients[indIp].Date = t.Format("2006-01-02 15:04:05 -0700 MST")
		}
	}
}

func isIgnored(conf int, alertConf int, err string) bool {
	for _, e := range config.Cfgs[conf].Alertlog[alertConf].Ignore {
		if e == err {
			return true
		}
	}
	return false
}

func addError(oerr string) {
	found := false
	for i := range Errors {
		if Errors[i].ora == oerr {
			Errors[i].count++
			found = true
			break
		}
	}
	if !found {
		err := oraerr{ora: oerr, count: 1}
		Errors = append(Errors, err)
	}
}

func (e *Exporter) ScrapeAlertlog() {
	loc := time.Now().Location()
	re := regexp.MustCompile(`O(RA|GG)-[0-9]+`)
	oggTimeRe := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})\s+(.*)$`)

	ReadAccess()
	for conf, _ := range config.Cfgs {
		var lastTime time.Time
		lastScrapeTime := e.GetLastScrapeTime(conf).Add(time.Second)

		for alertConfig, _ := range config.Cfgs[conf].Alertlog {
			Errors = nil
			info, _ := os.Stat(config.Cfgs[conf].Alertlog[alertConfig].File)
			file, err := os.Open(config.Cfgs[conf].Alertlog[alertConfig].File)
			ogg := config.Cfgs[conf].Alertlog[alertConfig].Ogg

			if err != nil {
				log.Errorln(err)
			} else {
				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					if ogg {
						match := oggTimeRe.FindStringSubmatch(scanner.Text())
						if match != nil {
							t, err := time.ParseInLocation(ogglayout, match[1], loc)
							if err == nil {
								lastTime = t
								if lastTime.After(lastScrapeTime) {
									if re.MatchString(scanner.Text()) {
										oerr := re.FindString(scanner.Text())
										if !isIgnored(conf, alertConfig, oerr) {
											addError(oerr)
										}
									}
								}
							} else {
								continue
							}
						}
					} else {
						t, err := time.ParseInLocation(oralayout, scanner.Text(), loc)
						if err == nil {
							lastTime = t
						} else {
							if lastTime.After(lastScrapeTime) {
								if re.MatchString(scanner.Text()) {
									oerr := re.FindString(scanner.Text())
									if !isIgnored(conf, alertConfig, oerr) {
										addError(oerr)
									}
								}
							}
						}
					}
				}
				file.Close()
				e.SetLastScrapeTime(conf, lastTime)
				for i, _ := range Errors {
					e.alertlog.WithLabelValues(
						config.Cfgs[conf].Database,
						config.Cfgs[conf].Instance,
						Errors[i].ora).Set(float64(Errors[i].count))
					WriteLog(config.Cfgs[conf].Instance + " " + e.lastIp +
						" (" + strconv.Itoa(Errors[i].count) + "): " +
						Errors[i].ora)
				}
				e.alertdate.WithLabelValues(config.Cfgs[conf].Database,
					config.Cfgs[conf].Instance).Set(float64(info.ModTime().Unix()))
			}
		}
	}
	WriteAccess()
}
