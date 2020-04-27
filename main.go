package main

import (
	"database/sql"
	"flag"
	"net"
	"net/http"
	"strconv"
	"time"

	_ "github.com/mattn/go-oci8"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
)

// Metric name parts.
const (
	namespace = "oracledb"
	exporter  = "exporter"
)

// Exporter collects Oracle DB metrics. It implements prometheus.Collector.
type Exporter struct {
	duration, error prometheus.Gauge
	totalScrapes    prometheus.Counter
	scrapeErrors    *prometheus.CounterVec
	session         *prometheus.GaugeVec
	sysstat         *prometheus.GaugeVec
	waitclass       *prometheus.GaugeVec
	sysmetric       *prometheus.GaugeVec
	interconnect    *prometheus.GaugeVec
	uptime          *prometheus.GaugeVec
	up              *prometheus.GaugeVec
	tablespace      *prometheus.GaugeVec
	recovery        *prometheus.GaugeVec
	redo            *prometheus.GaugeVec
	cache           *prometheus.GaugeVec
	alertlog        *prometheus.GaugeVec
	alertdate       *prometheus.GaugeVec
	services        *prometheus.GaugeVec
	parameter       *prometheus.GaugeVec
	//query           *prometheus.GaugeVec
	asmspace   *prometheus.GaugeVec
	tablerows  *prometheus.GaugeVec
	tablebytes *prometheus.GaugeVec
	indexbytes *prometheus.GaugeVec
	lobbytes   *prometheus.GaugeVec
	lastIp     string
	vTabRows   bool
	vTabBytes  bool
	vIndBytes  bool
	vLobBytes  bool
	vRecovery  bool
	custom     map[string]*prometheus.GaugeVec
}

var (
	// Version will be set at build time.
	Version       = "1.1.5"
	listenAddress = flag.String("web.listen-address", ":9161", "Address to listen on for web interface and telemetry.")
	metricPath    = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	pMetrics      = flag.Bool("defaultmetrics", true, "Expose standard metrics")
	pTabRows      = flag.Bool("tablerows", false, "Expose Table rows (CAN TAKE VERY LONG)")
	pTabBytes     = flag.Bool("tablebytes", false, "Expose Table size (CAN TAKE VERY LONG)")
	pIndBytes     = flag.Bool("indexbytes", false, "Expose Index size for any Table (CAN TAKE VERY LONG)")
	pLobBytes     = flag.Bool("lobbytes", false, "Expose Lobs size for any Table (CAN TAKE VERY LONG)")
	pRecovery     = flag.Bool("recovery", false, "Expose Recovery percentage usage of FRA (CAN TAKE VERY LONG)")
	configFile    = flag.String("configfile", "oracle.conf", "ConfigurationFile in YAML format.")
	logFile       = flag.String("logfile", "exporter.log", "Logfile for parsed Oracle Alerts.")
	accessFile    = flag.String("accessfile", "access.conf", "Last access for parsed Oracle Alerts.")
	landingPage   = []byte(`<html>
                          <head><title>Prometheus Oracle exporter</title></head>
                          <body>
                            <h1>Prometheus Oracle exporter</h1><p>
                            <a href='` + *metricPath + `'>Metrics</a></p>
                            <a href='` + *metricPath + `?tablerows=true'>Metrics with tablerows</a></p>
                            <a href='` + *metricPath + `?tablebytes=true'>Metrics with tablebytes</a></p>
                            <a href='` + *metricPath + `?indexbytes=true'>Metrics with indexbytes</a></p>
                            <a href='` + *metricPath + `?lobbytes=true'>Metrics with lobbytes</a></p>
                            <a href='` + *metricPath + `?recovery=true'>Metrics with recovery</a></p>
                          </body>
                          </html>`)
)

// NewExporter returns a new Oracle DB exporter for the provided DSN.
func NewExporter() *Exporter {
	e := Exporter{
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
		}, []string{"database", "dbinstance", "type"}),
		waitclass: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "waitclass",
			Help:      "Gauge metric with Waitevents (v$waitclassmetric).",
		}, []string{"database", "dbinstance", "type"}),
		sysstat: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "sysstat",
			Help:      "Gauge metric with commits/rollbacks/parses (v$sysstat).",
		}, []string{"database", "dbinstance", "type"}),
		session: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "session",
			Help:      "Gauge metric user/system active/passive sessions (v$session).",
		}, []string{"database", "dbinstance", "type", "state"}),
		uptime: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "uptime",
			Help:      "Gauge metric with uptime in days of the Instance.",
		}, []string{"database", "dbinstance"}),
		tablespace: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "tablespace",
			Help:      "Gauge metric with total/free size of the Tablespaces.",
		}, []string{"database", "dbinstance", "type", "name", "contents", "autoextend"}),
		interconnect: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "interconnect",
			Help:      "Gauge metric with interconnect block transfers (v$sysstat).",
		}, []string{"database", "dbinstance", "type"}),
		recovery: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "recovery",
			Help:      "Gauge metric with percentage usage of FRA (v$recovery_file_dest).",
		}, []string{"database", "dbinstance", "type"}),
		redo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "redo",
			Help:      "Gauge metric with Redo log switches over last 5 min (v$log_history).",
		}, []string{"database", "dbinstance"}),
		cache: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cachehitratio",
			Help:      "Gauge metric witch Cache hit ratios (v$sysmetric).",
		}, []string{"database", "dbinstance", "type"}),
		up: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Whether the Oracle server is up.",
		}, []string{"database", "dbinstance"}),
		alertlog: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "error",
			Help:      "Oracle Errors occured during configured interval.",
		}, []string{"database", "dbinstance", "code", "description", "ignore"}),
		alertdate: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "error_unix_seconds",
			Help:      "Unixtime of Alertlog modified Date.",
		}, []string{"database", "dbinstance"}),
		services: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "services",
			Help:      "Active Oracle Services (v$active_services).",
		}, []string{"database", "dbinstance", "name"}),
		parameter: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "parameter",
			Help:      "oracle Configuration Parameters (v$parameter).",
		}, []string{"database", "dbinstance", "name"}),
		// query: prometheus.NewGaugeVec(prometheus.GaugeOpts{
		// 	Namespace: namespace,
		// 	Name:      "query",
		// 	Help:      "Self defined Queries from Configuration File.",
		// }, []string{"database", "dbinstance", "name", "column", "row"}),
		asmspace: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "asmspace",
			Help:      "Gauge metric with total/free size of the ASM Diskgroups.",
		}, []string{"database", "dbinstance", "type", "name"}),
		tablerows: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "tablerows",
			Help:      "Gauge metric with rows of all Tables.",
		}, []string{"database", "dbinstance", "owner", "table_name", "tablespace"}),
		tablebytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "tablebytes",
			Help:      "Gauge metric with bytes of all Tables.",
		}, []string{"database", "dbinstance", "owner", "table_name"}),
		indexbytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "indexbytes",
			Help:      "Gauge metric with bytes of all Indexes per Table.",
		}, []string{"database", "dbinstance", "owner", "table_name"}),
		lobbytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "lobbytes",
			Help:      "Gauge metric with bytes of all Lobs per Table.",
		}, []string{"database", "dbinstance", "owner", "table_name"}),
		custom: make(map[string]*prometheus.GaugeVec),
	}
	// add custom metrics
	for _, conn := range config.Cfgs {
		for _, query := range conn.Queries {
			labels := []string{}
			for _, label := range query.Labels {
				labels = append(labels, cleanName(label))
			}
			e.custom[query.Name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "custom_" + cleanName(query.Name),
				Help:      query.Help,
			}, append(labels, "metric", "database", "dbinstance", "rownum"))
		}
	}

	return &e
}

// ScrapeCustomQueries collects metrics from self defined queries from configuration file.
func (e *Exporter) ScrapeCustomQueries() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			for _, query := range conn.Queries {
				rows, err = conn.db.Query(query.Sql)
				if err != nil {
					continue
				}

				cols, _ := rows.Columns()
				vals := make([]interface{}, len(cols))

				defer rows.Close()
				var rownum int = 1

				for rows.Next() {
					for i := range cols {
						vals[i] = &vals[i]
					}

					err = rows.Scan(vals...)
					if err != nil {
						break
					}

				MetricLoop:
					for _, metric := range query.Metrics {
						metricColumnIndex := -1
						for i, col := range cols {
							if cleanName(metric) == cleanName(col) {
								metricColumnIndex = i
								break
							}
						}

						if metricColumnIndex == -1 {
							//log.Infoln("Metric column '" + metric + "' not found")
							continue MetricLoop
						}

						if metricValue, ok := vals[metricColumnIndex].(float64); ok {
							promLabels := prometheus.Labels{}
							promLabels["database"] = conn.Database
							promLabels["dbinstance"] = conn.Instance
							promLabels["metric"] = metric
							promLabels["rownum"] = strconv.Itoa(rownum)
						LebelLoop:
							for _, label := range query.Labels {
								labelColumnIndex := -1
								for i, col := range cols {
									if cleanName(label) == cleanName(col) {
										labelColumnIndex = i
										break
									}
								}

								if labelColumnIndex == -1 {
									//log.Infoln("Label column not found")
									break LebelLoop
								}

								if a, ok := vals[labelColumnIndex].(string); ok {
									promLabels[cleanName(label)] = a
								} else if b, ok := vals[labelColumnIndex].(float64); ok {
									// if value is integer
									if b == float64(int64(b)) {
										promLabels[cleanName(label)] = strconv.Itoa(int(b))
									} else {
										promLabels[cleanName(label)] = strconv.FormatFloat(b, 'e', -1, 64)
									}
								}
							}
							e.custom[query.Name].With(promLabels).Set(metricValue)
						}
					}

					rownum++
				}
			}
		}
	}
}

// ScrapeQuery collects metrics from self defined queries from configuration file.
// func (e *Exporter) ScrapeQuery() {
// 	var (
// 		rows *sql.Rows
// 		err  error
// 	)
// 	for _, conn := range config.Cfgs {
// 		if conn.db != nil {
// 			for _, query := range conn.Queries {
// 				rows, err = conn.db.Query(query.Sql)
// 				if err != nil {
// 					continue
// 				}

// 				cols, _ := rows.Columns()
// 				vals := make([]interface{}, len(cols))
// 				var rownum int = 1

// 				defer rows.Close()
// 				for rows.Next() {
// 					for i := range cols {
// 						vals[i] = &vals[i]
// 					}

// 					err = rows.Scan(vals...)
// 					if err != nil {
// 						break
// 					}

// 					for i := range cols {
// 						if value, ok := vals[i].(float64); ok {
// 							e.query.WithLabelValues(conn.Database, conn.Instance, query.Name, cols[i], strconv.Itoa(rownum)).Set(value)
// 						}
// 					}
// 					rownum++
// 				}
// 			}
// 		}
// 	}
// }

// ScrapeParameters collects metrics from the v$parameters view.
func (e *Exporter) ScrapeParameter() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		//num  metric_name
		//43  sessions
		if conn.db != nil {
			rows, err = conn.db.Query(`select name,value from v$parameter WHERE num=43`)
			if err != nil {
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var value float64
				if err := rows.Scan(&name, &value); err != nil {
					break
				}
				name = cleanName(name)
				e.parameter.WithLabelValues(conn.Database, conn.Instance, name).Set(value)
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
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				if err := rows.Scan(&name); err != nil {
					break
				}
				name = cleanName(name)
				e.services.WithLabelValues(conn.Database, conn.Instance, name).Set(1)
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
		//metric_id  metric_name
		//2000    Buffer Cache Hit Ratio
		//2050    Cursor Cache Hit Ratio
		//2112    Library Cache Hit Ratio
		//2110    Row Cache Hit Ratio
		if conn.db != nil {
			rows, err = conn.db.Query(`select metric_name,value
                                 from v$sysmetric
                                 where group_id=2 and metric_id in (2000,2050,2112,2110)`)
			if err != nil {
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var value float64
				if err := rows.Scan(&name, &value); err != nil {
					break
				}
				name = cleanName(name)
				e.cache.WithLabelValues(conn.Database, conn.Instance, name).Set(value)
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
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var value float64
				if err := rows.Scan(&value); err != nil {
					break
				}
				e.redo.WithLabelValues(conn.Database, conn.Instance).Set(value)
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
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var used float64
				var recl float64
				if err := rows.Scan(&used, &recl); err != nil {
					break
				}
				e.recovery.WithLabelValues(conn.Database, conn.Instance, "percent_space_used").Set(used)
				e.recovery.WithLabelValues(conn.Database, conn.Instance, "percent_space_reclaimable").Set(recl)
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
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var value float64
				if err := rows.Scan(&name, &value); err != nil {
					break
				}
				name = cleanName(name)
				e.interconnect.WithLabelValues(conn.Database, conn.Instance, name).Set(value)
			}
		}
	}
}

// ScrapeAsmspace collects ASM metrics
func (e *Exporter) ScrapeAsmspace() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			rows, err = conn.db.Query(`SELECT g.name, sum(d.total_mb), sum(d.free_mb)
                                  FROM v$asm_disk_stat d, v$asm_diskgroup_stat g
                                 WHERE  d.group_number = g.group_number
                                  AND  d.header_status = 'MEMBER'
                                 GROUP by  g.name,  g.group_number`)
			if err != nil {
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var tsize float64
				var tfree float64
				if err := rows.Scan(&name, &tsize, &tfree); err != nil {
					break
				}
				e.asmspace.WithLabelValues(conn.Database, conn.Instance, "total", name).Set(tsize)
				e.asmspace.WithLabelValues(conn.Database, conn.Instance, "free", name).Set(tfree)
				e.asmspace.WithLabelValues(conn.Database, conn.Instance, "used", name).Set(tsize - tfree)
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
                                   getsize AS (SELECT tablespace_name, max(autoextensible) autoextensible, SUM(bytes) tsize
                                               FROM dba_data_files GROUP BY tablespace_name),
                                   getfree as (SELECT tablespace_name, contents, SUM(blocks*block_size) tfree
                                               FROM DBA_LMT_FREE_SPACE a, v$tablespace b, dba_tablespaces c
                                               WHERE a.TABLESPACE_ID= b.ts# and b.name=c.tablespace_name
                                               GROUP BY tablespace_name,contents)
                                 SELECT a.tablespace_name, b.contents, a.tsize,  b.tfree, a.autoextensible autoextend
                                 FROM GETSIZE a, GETFREE b
                                 WHERE a.tablespace_name = b.tablespace_name
                                 UNION
                                 SELECT tablespace_name, 'TEMPORARY', sum(tablespace_size), sum(free_space), 'NO'
                                 FROM dba_temp_free_space
                                 GROUP BY tablespace_name`)
			if err != nil {
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var contents string
				var tsize float64
				var tfree float64
				var auto string
				if err := rows.Scan(&name, &contents, &tsize, &tfree, &auto); err != nil {
					break
				}
				e.tablespace.WithLabelValues(conn.Database, conn.Instance, "total", name, contents, auto).Set(tsize)
				e.tablespace.WithLabelValues(conn.Database, conn.Instance, "free", name, contents, auto).Set(tfree)
				e.tablespace.WithLabelValues(conn.Database, conn.Instance, "used", name, contents, auto).Set(tsize - tfree)
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
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var user string
				var status string
				var value float64
				if err := rows.Scan(&user, &status, &value); err != nil {
					break
				}
				e.session.WithLabelValues(conn.Database, conn.Instance, user, status).Set(value)
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
			e.uptime.WithLabelValues(conn.Database, conn.Instance).Set(uptime)
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
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var value float64
				if err := rows.Scan(&name, &value); err != nil {
					break
				}
				name = cleanName(name)
				e.sysstat.WithLabelValues(conn.Database, conn.Instance, name).Set(value)
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
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var value float64
				if err := rows.Scan(&name, &value); err != nil {
					break
				}
				name = cleanName(name)
				e.waitclass.WithLabelValues(conn.Database, conn.Instance, name).Set(value)
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
		//metric_id  metric_name
		//2092    Physical Read Total IO Requests Per Sec
		//2093    Physical Read Total Bytes Per Sec
		//2100    Physical Write Total IO Requests Per Sec
		//2124    Physical Write Total Bytes Per Sec
		if conn.db != nil {
			rows, err = conn.db.Query("select metric_name,value from v$sysmetric where metric_id in (2092,2093,2124,2100)")
			if err != nil {
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var value float64
				if err := rows.Scan(&name, &value); err != nil {
					break
				}
				name = cleanName(name)
				e.sysmetric.WithLabelValues(conn.Database, conn.Instance, name).Set(value)
			}
		}
	}
}

// ScrapeTablerows collects bytes from dba_tables view.
func (e *Exporter) ScrapeTablerows() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			rows, err = conn.db.Query(`select owner,table_name, tablespace_name, num_rows
                                 from dba_tables
                                 where owner not like '%SYS%' and num_rows is not null`)
			if err != nil {
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var owner string
				var name string
				var space string
				var value float64
				if err := rows.Scan(&owner, &name, &space, &value); err != nil {
					break
				}
				name = cleanName(name)
				e.tablerows.WithLabelValues(conn.Database, conn.Instance, owner, name, space).Set(value)
			}
		}
	}
}

func (e *Exporter) ScrapeTablebytes() {
	// ScrapeTablebytes collects bytes from dba_tables/dba_segments view.
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			rows, err = conn.db.Query(`SELECT tab.owner, tab.table_name,  stab.bytes
                                 FROM dba_tables  tab, dba_segments stab
                                 WHERE stab.owner = tab.owner AND stab.segment_name = tab.table_name
                                 AND tab.owner NOT LIKE '%SYS%'`)
			if err != nil {
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var owner string
				var name string
				var value float64
				if err = rows.Scan(&owner, &name, &value); err != nil {
					break
				}
				name = cleanName(name)
				e.tablebytes.WithLabelValues(conn.Database, conn.Instance, owner, name).Set(value)
			}
		}
	}
}

// ScrapeTablebytes collects bytes from dba_indexes/dba_segments view.
func (e *Exporter) ScrapeIndexbytes() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			rows, err = conn.db.Query(`select table_owner,table_name, sum(bytes)
                                 from dba_indexes ind, dba_segments seg
                                 WHERE ind.owner=seg.owner and ind.index_name=seg.segment_name
                                 and table_owner NOT LIKE '%SYS%'
                                 group by table_owner,table_name`)
			if err != nil {
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var owner string
				var name string
				var value float64
				if err = rows.Scan(&owner, &name, &value); err != nil {
					break
				}
				name = cleanName(name)
				e.indexbytes.WithLabelValues(conn.Database, conn.Instance, owner, name).Set(value)
			}
		}
	}
}

// ScrapeLobbytes collects bytes from dba_lobs/dba_segments view.
func (e *Exporter) ScrapeLobbytes() {
	var (
		rows *sql.Rows
		err  error
	)
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			rows, err = conn.db.Query(`select l.owner, l.table_name, sum(bytes)
                                 from dba_lobs l, dba_segments seg
                                 WHERE l.owner=seg.owner and l.table_name=seg.segment_name
                                 and l.owner NOT LIKE '%SYS%'
                                 group by l.owner,l.table_name`)
			if err != nil {
				continue
			}
			defer rows.Close()
			for rows.Next() {
				var owner string
				var name string
				var value float64
				if err = rows.Scan(&owner, &name, &value); err != nil {
					break
				}
				name = cleanName(name)
				e.lobbytes.WithLabelValues(conn.Database, conn.Instance, owner, name).Set(value)
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
	e.alertlog.Describe(ch)
	e.alertdate.Describe(ch)
	e.services.Describe(ch)
	e.parameter.Describe(ch)
	//e.query.Describe(ch)
	e.asmspace.Describe(ch)
	e.tablerows.Describe(ch)
	e.tablebytes.Describe(ch)
	e.indexbytes.Describe(ch)
	e.lobbytes.Describe(ch)
	for _, metric := range e.custom {
		metric.Describe(ch)
	}
}

// Connect the DBs and gather Databasename and Instancename
func (e *Exporter) Connect() {
	var dbname string
	var inname string
	var err error

	for i, conf := range config.Cfgs {
		// Close Connect from former scrape that not closed properly
		if config.Cfgs[i].db != nil {
			config.Cfgs[i].db.Close()
			config.Cfgs[i].db = nil
		}
		if len(conf.Connection) > 0 {
			config.Cfgs[i].db, err = sql.Open("oci8", conf.Connection)
			if err == nil {
				err = config.Cfgs[i].db.QueryRow("select db_unique_name,instance_name from v$database,v$instance").Scan(&dbname, &inname)
				if err == nil {
					if (len(conf.Database) == 0) || (len(conf.Instance) == 0) {
						config.Cfgs[i].Database = dbname
						config.Cfgs[i].Instance = inname
					}
					e.up.WithLabelValues(conf.Database, conf.Instance).Set(1)
				} else {
					config.Cfgs[i].db.Close()
					e.up.WithLabelValues(conf.Database, conf.Instance).Set(0)
					log.Errorln("Error connecting to database:", err)
					//log.Infoln("Connect OK, Inital query failed: ", conf.Connection)
				}
			}
		} else {
			//log.Infoln("Dummy Connection: ", conf.Database)
			e.up.WithLabelValues(conf.Database, conf.Instance).Set(0)
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
	e.alertlog.Reset()
	e.alertdate.Reset()
	e.services.Reset()
	e.parameter.Reset()
	//e.query.Reset()
	e.asmspace.Reset()
	e.tablerows.Reset()
	e.tablebytes.Reset()
	e.indexbytes.Reset()
	e.lobbytes.Reset()

	for _, metric := range e.custom {
		metric.Reset()
	}
}

// Close Connections
func (e *Exporter) Close() {
	for _, conn := range config.Cfgs {
		if conn.db != nil {
			conn.db.Close()
			conn.db = nil
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

	if e.vRecovery || *pRecovery {
		e.ScrapeRecovery()
		e.recovery.Collect(ch)
	}

	if *pMetrics {
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

		e.ScrapeRedo()
		e.redo.Collect(ch)

		e.ScrapeCache()
		e.cache.Collect(ch)

		e.ScrapeAlertlog()
		e.alertlog.Collect(ch)
		e.alertdate.Collect(ch)

		e.ScrapeServices()
		e.services.Collect(ch)

		e.ScrapeParameter()
		e.parameter.Collect(ch)

		e.ScrapeAsmspace()
		e.asmspace.Collect(ch)
	}

	e.ScrapeCustomQueries()
	for _, metric := range e.custom {
		metric.Collect(ch)
	}
	//e.ScrapeQuery()
	//e.query.Collect(ch)

	if e.vTabRows || *pTabRows {
		e.ScrapeTablerows()
		e.tablerows.Collect(ch)
	}

	if e.vTabBytes || *pTabBytes {
		e.ScrapeTablebytes()
		e.tablebytes.Collect(ch)
	}

	if e.vIndBytes || *pIndBytes {
		e.ScrapeIndexbytes()
		e.indexbytes.Collect(ch)
	}

	if e.vLobBytes || *pLobBytes {
		e.ScrapeLobbytes()
		e.lobbytes.Collect(ch)
	}

	ch <- e.duration
	ch <- e.totalScrapes
	ch <- e.error
	e.scrapeErrors.Collect(ch)

	e.Close()
}

func (e *Exporter) Handler(w http.ResponseWriter, r *http.Request) {
	e.lastIp = ""
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		e.lastIp = ip
	}
	e.vTabRows = false
	e.vTabBytes = false
	e.vIndBytes = false
	e.vLobBytes = false
	e.vRecovery = false
	if r.URL.Query().Get("tablerows") == "true" {
		e.vTabRows = true
	}
	if r.URL.Query().Get("tablebytes") == "true" {
		e.vTabBytes = true
	}
	if r.URL.Query().Get("indexbytes") == "true" {
		e.vIndBytes = true
	}
	if r.URL.Query().Get("lobbytes") == "true" {
		e.vLobBytes = true
	}
	if r.URL.Query().Get("recovery") == "true" {
		e.vRecovery = true
	}
	promhttp.Handler().ServeHTTP(w, r)
}

func main() {
	flag.Parse()
	log.Infoln("Starting Prometheus Oracle exporter " + Version)
	if loadConfig() {
		log.Infoln("Config loaded: ", *configFile)
		exporter := NewExporter()
		prometheus.MustRegister(exporter)

		http.HandleFunc(*metricPath, exporter.Handler)

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.Write(landingPage) })

		log.Infoln("Listening on", *listenAddress)
		log.Fatal(http.ListenAndServe(*listenAddress, nil))
	}
}
