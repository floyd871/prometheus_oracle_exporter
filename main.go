package main

import (
		"database/sql"
		"flag"
		"net/http"
		"time"
	_ "github.com/mattn/go-oci8"
		"github.com/prometheus/client_golang/prometheus"
		"github.com/prometheus/common/log"
)

// Metric name parts.
const (
	namespace = "oracledb"
	exporter  = "exporter"
)

type Alert struct {
	File string    		 `yaml:"file"`
	Scantime int       `yaml:"scantime"`
	Ignoreora []string `yaml:"ignoreora"`
}

type Config struct {
	Connection string  `yaml:"connection"`
	Database string    `yaml:"database"`
	Instance string    `yaml:"instance"`
	Alertlog []Alert 	 `yaml:"alertlog"`
	db                 *sql.DB
}

type Configs struct {
	Cfgs []Config `yaml:"connections"`
}

// Exporter collects Oracle DB metrics. It implements prometheus.Collector.
type Exporter struct {
	duration, error prometheus.Gauge
	totalScrapes    prometheus.Counter
	scrapeErrors    *prometheus.CounterVec
  session         *prometheus.GaugeVec
  sysstat         *prometheus.GaugeVec
  waitclass       *prometheus.GaugeVec
	sysmetric	    	*prometheus.GaugeVec
	interconnect		*prometheus.GaugeVec
	uptime          *prometheus.GaugeVec
	up		        	*prometheus.GaugeVec
	tablespace      *prometheus.GaugeVec
	recovery        *prometheus.GaugeVec
	redo            *prometheus.GaugeVec
	cache           *prometheus.GaugeVec
	oraerror        *prometheus.GaugeVec
  services        *prometheus.GaugeVec
	parameter       *prometheus.GaugeVec
}

var (
	// Version will be set at build time.
	Version       = "1.0.0"
	listenAddress = flag.String("web.listen-address", ":9161", "Address to listen on for web interface and telemetry.")
	metricPath    = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	configFile    = flag.String("configfile", "oracle.conf", "ConfigurationFile in YAML format.")
	landingPage   = []byte("<html><head><title>Prometheus Oracle exporter</title></head><body><h1>Prometheus Oracle exporter</h1><p><a href='" + *metricPath + "'>Metrics</a></p></body></html>")
	config 				Configs
)


// NewExporter returns a new Oracle DB exporter for the provided DSN.
func NewExporter() *Exporter {
	return &Exporter{
		duration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporter,
			Name:      "last_scrape_duration_seconds",
			Help:      "Duration of the last scrape of metrics from Oracle DB.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: exporter,
			Name:      "scrapes_total",
			Help:      "Total number of times Oracle DB was scraped for metrics.",
		}),
		scrapeErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: exporter,
			Name:      "scrape_errors_total",
			Help:      "Total number of times an error occured scraping a Oracle database.",
		}, []string{"collector"}),
		error: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporter,
			Name:      "last_scrape_error",
			Help:      "Whether the last scrape of metrics from Oracle DB resulted in an error (1 for error, 0 for success).",
		}),
		sysmetric: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "sysmetric",
			Help:      "Gauge metric with read/write pysical IOPs/bytes (v$sysmetric).",
		}, []string{"database","dbinstance","type"}),
		waitclass: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "waitclass",
			Help:      "Gauge metric with Waitevents (v$waitclassmetric).",
		}, []string{"database","dbinstance","type"}),
		sysstat: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "sysstat",
			Help:      "Gauge metric with commits/rollbacks/parses (v$sysstat).",
		}, []string{"database","dbinstance","type"}),
		session: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "session",
			Help:      "Gauge metric user/system active/passive sessions (v$session).",
		}, []string{"database","dbinstance","type","state"}),
		uptime: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "uptime",
			Help:      "Gauge metric with uptime in days of the Instance.",
		}, []string{"database","dbinstance"}),
		tablespace: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "tablespace",
			Help:      "Gauge metric with total/free size of the Tablespaces.",
		}, []string{"database","dbinstance","type","name","contents"}),
		interconnect: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "interconnect",
			Help:      "Gauge metric with interconnect block transfers (v$sysstat).",
		}, []string{"database","dbinstance","type"}),
		recovery: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "recovery",
			Help:      "Gauge metric with percentage usage of FRA (v$recovery_file_dest).",
		}, []string{"database","dbinstance","type"}),
		redo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "redo",
			Help:      "Gauge metric with Redo log switches over last 5 min (v$log_history).",
		}, []string{"database","dbinstance"}),
		cache: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cachehitratio",
			Help:      "Gauge metric witch Cache hit ratios (v$sysmetric).",
		}, []string{"database","dbinstance","type"}),
		up: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Whether the Oracle server is up.",
		}, []string{"database","dbinstance"}),
		oraerror: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "error",
			Help:      "Oracle Errors occured during configured interval.",
		}, []string{"database","dbinstance","type","name"}),
		services: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "services",
			Help:      "Active Oracle Services (v$active_services).",
		}, []string{"database","dbinstance","name"}),
		parameter: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "parameter",
			Help:      "oracle Configuration Parameters (v$parameter).",
		}, []string{"database","dbinstance","name"}),
	}
}

// ScrapeParameters collects metrics from the v$parameters view.
func (e *Exporter) ScrapeParameter() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		//num	metric_name
		//43	sessions
		if conn.db != nil {
			rows, err = conn.db.Query(`select name,value from v$parameter WHERE num=43`)
			if err != nil {
				break
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var value float64
				if err := rows.Scan(&name,&value); err != nil {
					break
				}
				name = cleanName(name)
		    e.parameter.WithLabelValues(conn.Database,conn.Instance,name).Set(value)
			}
		}
	}
}


// ScrapeServices collects metrics from the v$active_services view.
func (e *Exporter) ScrapeServices() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			rows, err = conn.db.Query(`select name from v$active_services`)
			if err != nil {
				break
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				if err := rows.Scan(&name); err != nil {
					break
				}
				name = cleanName(name)
		    e.services.WithLabelValues(conn.Database,conn.Instance,name).Set(1)
			}
		}
	}
}


// ScrapeCache collects session metrics from the v$sysmetrics view.
func (e *Exporter) ScrapeCache() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		//metric_id	metric_name
		//2000		Buffer Cache Hit Ratio
		//2050		Cursor Cache Hit Ratio
		//2112		Library Cache Hit Ratio
		//2110		Row Cache Hit Ratio
		if conn.db != nil {
			rows, err = conn.db.Query(`select metric_name,value
	                               from v$sysmetric
	                               where group_id=2 and metric_id in (2000,2050,2112,2110)`)
			if err != nil {
				break
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var value float64
				if err := rows.Scan(&name, &value); err != nil {
					break
				}
				name = cleanName(name)
		    e.cache.WithLabelValues(conn.Database,conn.Instance,name).Set(value)
			}
		}
	}
}


// ScrapeRecovery collects tablespace metrics
func (e *Exporter) ScrapeRedo() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			rows, err = conn.db.Query(`select count(*) from v$log_history where first_time > sysdate - 1/24/12`)
			if err != nil {
				break
			}
			defer rows.Close()
			for rows.Next() {
				var value float64
				if err := rows.Scan(&value); err != nil {
					break
				}
	      e.redo.WithLabelValues(conn.Database,conn.Instance).Set(value)
			}
		}
	}
}

// ScrapeRecovery collects tablespace metrics
func (e *Exporter) ScrapeRecovery() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			rows, err = conn.db.Query(`SELECT sum(percent_space_used) , sum(percent_space_reclaimable)
	                               from V$FLASH_RECOVERY_AREA_USAGE`)
			if err != nil {
				break
			}
			defer rows.Close()
			for rows.Next() {
				var used float64
				var recl float64
				if err := rows.Scan(&used, &recl); err != nil {
					break
				}
	      e.recovery.WithLabelValues(conn.Database,conn.Instance,"percent_space_used").Set(used)
	      e.recovery.WithLabelValues(conn.Database,conn.Instance,"percent_space_reclaimable").Set(recl)
			}
		}
	}
}

// ScrapeTablespaces collects tablespace metrics
func (e *Exporter) ScrapeInterconnect() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			rows, err = conn.db.Query(`SELECT name, value
	                               FROM V$SYSSTAT
	                               WHERE name in ('gc cr blocks served','gc cr blocks flushed','gc cr blocks received')`)
			if err != nil {
				break
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var value float64
				if err := rows.Scan(&name, &value); err != nil {
					break
				}
				name = cleanName(name)
		    e.interconnect.WithLabelValues(conn.Database,conn.Instance,name).Set(value)
			}
		}
	}
}

// ScrapeTablespaces collects tablespace metrics
func (e *Exporter) ScrapeTablespace() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			rows, err = conn.db.Query(`WITH
	                                 getsize AS (SELECT tablespace_name, SUM(bytes) tsize
	                                             FROM dba_data_files GROUP BY tablespace_name),
	                                 getfree as (SELECT tablespace_name, contents, SUM(blocks*block_size) tfree
	                                             FROM DBA_LMT_FREE_SPACE a, v$tablespace b, dba_tablespaces c
	                                             WHERE a.TABLESPACE_ID= b.ts# and b.name=c.tablespace_name
	                                             GROUP BY tablespace_name,contents)
	                               SELECT a.tablespace_name, b.contents, a.tsize,  b.tfree
	                               FROM GETSIZE a, GETFREE b
	                               WHERE a.tablespace_name = b.tablespace_name
						                     UNION
	                               SELECT tablespace_name, 'TEMPORARY', sum(tablespace_size), sum(free_space)
	                               FROM dba_temp_free_space
	                               GROUP BY tablespace_name`)
			if err != nil {
				break
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var contents string
				var tsize float64
				var tfree float64
				if err := rows.Scan(&name, &contents, &tsize, &tfree); err != nil {
					break
				}
		    e.tablespace.WithLabelValues(conn.Database,conn.Instance,"total",name,contents).Set(tsize)
		    e.tablespace.WithLabelValues(conn.Database,conn.Instance,"free",name,contents).Set(tfree)
		    e.tablespace.WithLabelValues(conn.Database,conn.Instance,"used",name,contents).Set(tsize-tfree)
			}
		}
	}
}

// ScrapeSessions collects session metrics from the v$session view.
func (e *Exporter) ScrapeSession() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			rows, err = conn.db.Query(`SELECT decode(username,NULL,'SYSTEM','SYS','SYSTEM','USER'), status,count(*)
	                               FROM v$session
	                               GROUP BY decode(username,NULL,'SYSTEM','SYS','SYSTEM','USER'),status`)
			if err != nil {
				break
			}
			defer rows.Close()
			for rows.Next() {
				var user string
				var status string
				var value float64
				if err := rows.Scan(&user, &status, &value); err != nil {
					break
				}
		    e.session.WithLabelValues(conn.Database,conn.Instance,user,status).Set(value)
			}
		}
	}
}


// ScrapeUptime Instance uptime
func (e *Exporter) ScrapeUptime() {
	var uptime float64
	for _, conn := range config.Cfgs {
		if conn.db != nil {
	    err := conn.db.QueryRow("select sysdate-startup_time from v$instance").Scan(&uptime)
			if err != nil {
				return
			}
		  e.uptime.WithLabelValues(conn.Database,conn.Instance).Set(uptime)
		}
	}
}

// ScrapeSysstat collects activity metrics from the v$sysstat view.
func (e *Exporter) ScrapeSysstat() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			rows, err = conn.db.Query(`SELECT name, value FROM v$sysstat
						   										 WHERE statistic# in (6,7,1084,1089)`)
			if err != nil {
				break
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var value float64
				if err := rows.Scan(&name, &value); err != nil {
					break
				}
				name = cleanName(name)
	      e.sysstat.WithLabelValues(conn.Database,conn.Instance,name).Set(value)
			}
		}
	}
}

// ScrapeWaitTime collects wait time metrics from the v$waitclassmetric view.
func (e *Exporter) ScrapeWaitclass() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			rows, err = conn.db.Query(`SELECT n.wait_class, round(m.time_waited/m.INTSIZE_CSEC,3)
	                                  FROM v$waitclassmetric  m, v$system_wait_class n
	                                  WHERE m.wait_class_id=n.wait_class_id and n.wait_class != 'Idle'`)
			if err != nil {
				break
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var value float64
				if err := rows.Scan(&name, &value); err != nil {
					break
				}
				name = cleanName(name)
		    e.waitclass.WithLabelValues(conn.Database,conn.Instance,name).Set(value)
			}
		}
	}
}

// ScrapeSysmetrics collects session metrics from the v$sysmetrics view.
func (e *Exporter) ScrapeSysmetric() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		//metric_id	metric_name
		//2092		Physical Read Total IO Requests Per Sec
		//2093		Physical Read Total Bytes Per Sec
		//2100		Physical Write Total IO Requests Per Sec
		//2124		Physical Write Total Bytes Per Sec
		if conn.db != nil {
			rows, err = conn.db.Query("select metric_name,value from v$sysmetric where metric_id in (2092,2093,2124,2100)")
			if err != nil {
				break
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var value float64
				if err := rows.Scan(&name, &value); err != nil {
					break
				}
				name = cleanName(name)
		    e.sysmetric.WithLabelValues(conn.Database,conn.Instance,name).Set(value)
			}
		}
	}
}


// Describe describes all the metrics exported by the Oracle exporter.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	e.duration.Describe(ch)
	e.totalScrapes.Describe(ch)
	e.scrapeErrors.Describe(ch)
  e.session.Describe(ch)
  e.sysstat.Describe(ch)
  e.waitclass.Describe(ch)
	e.sysmetric.Describe(ch)
	e.interconnect.Describe(ch)
  e.tablespace.Describe(ch)
  e.recovery.Describe(ch)
  e.redo.Describe(ch)
  e.cache.Describe(ch)
	e.uptime.Describe(ch)
	e.up.Describe(ch)
	e.oraerror.Describe(ch)
	e.services.Describe(ch)
	e.parameter.Describe(ch)
}

// Connect the DBs and gather Databasename and Instancename
func (e *Exporter) Connect() {
	var dbname string
	var inname string

	for i, conf := range config.Cfgs {
		config.Cfgs[i].db = nil
		db , err := sql.Open("oci8", conf.Connection)
		if err == nil {
			err = db.QueryRow("select db_unique_name,instance_name from v$database,v$instance").Scan(&dbname,&inname)
			if err == nil {
				if (conf.Database != dbname) || (conf.Instance != inname) {
				  config.Cfgs[i].Database = dbname
					config.Cfgs[i].Instance = inname
				}
				config.Cfgs[i].db = db
				e.up.WithLabelValues(conf.Database,conf.Instance).Set(1)
			} else {
				e.up.WithLabelValues(conf.Database,conf.Instance).Set(0)
			}
		} else {
			e.up.WithLabelValues(conf.Database,conf.Instance).Set(0)
		}
	}
  e.session.Reset()
  e.sysstat.Reset()
  e.waitclass.Reset()
	e.sysmetric.Reset()
	e.interconnect.Reset()
  e.tablespace.Reset()
  e.recovery.Reset()
  e.redo.Reset()
  e.cache.Reset()
	e.uptime.Reset()
	e.oraerror.Reset()
	e.services.Reset()
	e.parameter.Reset()
}

// Close Connections
func (e *Exporter) Close() {
	for _, conn := range config.Cfgs {
		if conn.db != nil {
  		conn.db.Close()
		}
	}
}


// Collect implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	var err error

	e.totalScrapes.Inc()
	defer func(begun time.Time) {
		e.duration.Set(time.Since(begun).Seconds())
		if err == nil {
			e.error.Set(0)
		} else {
			e.error.Set(1)
		}
	}(time.Now())

	e.Connect()
	e.up.Collect(ch)

  e.ScrapeUptime()
  e.uptime.Collect(ch)

  e.ScrapeSession()
  e.session.Collect(ch)

  e.ScrapeSysstat()
  e.sysstat.Collect(ch)

	e.ScrapeWaitclass()
	e.waitclass.Collect(ch)

	e.ScrapeSysmetric()
	e.sysmetric.Collect(ch)

	e.ScrapeTablespace()
	e.tablespace.Collect(ch)

	e.ScrapeInterconnect()
	e.interconnect.Collect(ch)

	e.ScrapeRecovery()
	e.recovery.Collect(ch)

	e.ScrapeRedo()
	e.redo.Collect(ch)

	e.ScrapeCache()
	e.cache.Collect(ch)

  e.ScrapeOraerror()
	e.oraerror.Collect(ch)

	e.ScrapeServices()
	e.services.Collect(ch)

	e.ScrapeParameter()
	e.parameter.Collect(ch)

	ch <- e.duration
	ch <- e.totalScrapes
	ch <- e.error
	e.scrapeErrors.Collect(ch)

  e.Close()
}

func main() {
	flag.Parse()
	log.Infoln("Starting Prometheus Oracle exporter " + Version)
	if loadConfig() {

		log.Infoln("Config loaded: ", *configFile)
		exporter := NewExporter()
		prometheus.MustRegister(exporter)
	  http.Handle(*metricPath, prometheus.Handler())
	  http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {w.Write(landingPage)})

	  log.Infoln("Listening on", *listenAddress)
	  log.Fatal(http.ListenAndServe(*listenAddress, nil))
	}
}
