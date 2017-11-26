# Oracle DB Exporter

A [Prometheus](https://prometheus.io/) exporter for Oracle. Insipred from (https://github.com/iamseth/oracledb_exporter). I'm a DBA , PRs welcomed.

The following metrics are exposed currently. Support for RAC (databasename and instancename added via lables)

- oracledb_exporter_last_scrape_duration_seconds
- oracledb_exporter_last_scrape_error
- oracledb_exporter_scrapes_total
- oracledb_uptime (days)
- oracledb_session (view v$session system/user active/passive)
- oracledb_sysmetric (view v$sysmetric)
- oracledb_sysstat (view v$sysstat)
- oracledb_waitclass (view v$waitclass)
- oracledb_tsfree (tablespace free)
- oracledb_tssize (tablespace total size)
...

# Installation

Ensure that the environment variable DATA_SOURCE_NAME is set correctly before starting. You can add multiple instances, if you run more than one instance on a host. It is even possible to run one Exporter for all your Databases, but this is not recommended. We use it in our Company because on one host multiple Instances are running.

```bash
export DATA_SOURCE_NAME="system/oracle@myhost1;system/oracle@myhost2;system/oracle@myhost3"
/path/to/binary -l log.level error -l web.listen-address 9161
```

## Usage

```bash
Usage of oracledb_exporter:
  -log.format value
       	If set use a syslog logger or JSON logging. Example: logger:syslog?appname=bob&local=7 or logger:stdout?json=true. Defaults to stderr.
  -log.level value
       	Only log messages with the given severity or above. Valid levels: [debug, info, warn, error, fatal].
  -web.listen-address string
       	Address to listen on for web interface and telemetry. (default ":9161")
  -web.telemetry-path string
       	Path under which to expose metrics. (default "/metrics")
```
