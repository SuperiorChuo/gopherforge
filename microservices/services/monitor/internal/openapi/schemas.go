package openapi

type routeContract struct {
	RequestSchema       string
	ResponseSchema      string
	QueryParams         []Parameter
	NoRequestBody       bool
	OptionalRequestBody bool
	UnavailableResponse bool
	NotFoundResponse    bool
}

func coreSchemas() map[string]Schema {
	schemas := map[string]Schema{
		"EmptyResponse": objectSchema(map[string]Schema{}, nil),
		"AlertMetricDefinition": objectSchema(map[string]Schema{
			"key":         stringSchema(),
			"title":       stringSchema(),
			"unit":        stringSchema(),
			"description": stringSchema(),
			"operators":   arraySchema(stringSchema()),
		}, []string{"key", "title", "unit", "description", "operators"}),
		"AlertMetricList": objectSchema(map[string]Schema{
			"list": arraySchema(refSchema("AlertMetricDefinition")),
		}, []string{"list"}),
		"MonitorAlertSummary": objectSchema(map[string]Schema{
			"total":      integerSchema(),
			"enabled":    integerSchema(),
			"firing":     integerSchema(),
			"pending":    integerSchema(),
			"error":      integerSchema(),
			"checked_at": dateTimeSchema(),
		}, []string{"total", "enabled", "firing", "pending", "error", "checked_at"}),
		"MonitorAlertRule": objectSchema(map[string]Schema{
			"id":                integerSchema(),
			"name":              stringSchema(),
			"metric":            stringSchema(),
			"operator":          stringSchema(),
			"threshold":         numberSchema(),
			"duration_seconds":  integerSchema(),
			"severity":          stringSchema(),
			"enabled":           booleanSchema(),
			"notify_on_resolve": booleanSchema(),
			"state":             stringSchema(),
			"pending_since":     dateTimeSchema(),
			"firing_since":      dateTimeSchema(),
			"last_value":        numberSchema(),
			"last_evaluated_at": dateTimeSchema(),
			"last_error":        stringSchema(),
			"created_at":        dateTimeSchema(),
			"updated_at":        dateTimeSchema(),
		}, []string{"id", "name", "metric", "operator", "threshold", "duration_seconds", "severity", "enabled", "notify_on_resolve", "state", "created_at", "updated_at"}),
		"SaveAlertRuleRequest": objectSchema(map[string]Schema{
			"name":              stringSchema(),
			"metric":            stringSchema(),
			"operator":          stringSchema(),
			"threshold":         numberSchema(),
			"duration_seconds":  integerSchema(),
			"severity":          stringSchema(),
			"enabled":           booleanSchema(),
			"notify_on_resolve": booleanSchema(),
		}, []string{"name", "metric", "operator", "threshold"}),
		"AlertRuleListResponse": pageSchema(refSchema("MonitorAlertRule")),
		"MonitorAlertEvent": objectSchema(map[string]Schema{
			"id":            integerSchema(),
			"rule_id":       integerSchema(),
			"rule_name":     stringSchema(),
			"metric":        stringSchema(),
			"severity":      stringSchema(),
			"status":        stringSchema(),
			"value":         numberSchema(),
			"threshold":     numberSchema(),
			"message":       stringSchema(),
			"notify_status": stringSchema(),
			"notify_error":  stringSchema(),
			"notified_at":   dateTimeSchema(),
			"created_at":    dateTimeSchema(),
		}, []string{"id", "rule_name", "metric", "severity", "status", "value", "threshold", "message", "notify_status", "created_at"}),
		"AlertEventListResponse": pageSchema(refSchema("MonitorAlertEvent")),
		"AlertEvaluationResult": objectSchema(map[string]Schema{
			"rule":  refSchema("MonitorAlertRule"),
			"event": refSchema("MonitorAlertEvent"),
		}, []string{"rule"}),
		"ScheduledJob": objectSchema(map[string]Schema{
			"id":              integerSchema(),
			"name":            stringSchema(),
			"group_name":      stringSchema(),
			"cron_expression": stringSchema(),
			"invoke_target":   stringSchema(),
			"description":     stringSchema(),
			"status":          integerSchema(),
			"concurrent":      integerSchema(),
			"last_run_time":   dateTimeSchema(),
			"next_run_time":   dateTimeSchema(),
			"created_at":      dateTimeSchema(),
			"updated_at":      dateTimeSchema(),
		}, []string{"id", "name", "cron_expression", "status"}),
		"JobListResponse": pageSchema(refSchema("ScheduledJob")),
		"SaveJobRequest": objectSchema(map[string]Schema{
			"name":            stringSchema(),
			"group_name":      stringSchema(),
			"cron_expression": stringSchema(),
			"invoke_target":   stringSchema(),
			"description":     stringSchema(),
			"status":          integerSchema(),
			"concurrent":      integerSchema(),
		}, []string{"name", "cron_expression", "invoke_target"}),
		"JobLogCleanupRequest": objectSchema(map[string]Schema{
			"retention_days": minimumIntegerSchema(1),
		}, nil),
		"JobLogCleanupResult": objectSchema(map[string]Schema{
			"retention_days": integerSchema(),
			"cutoff_time":    dateTimeSchema(),
			"deleted_rows":   integerSchema(),
		}, []string{"retention_days", "cutoff_time", "deleted_rows"}),
		"JobAbnormalStatus": objectSchema(map[string]Schema{
			"id":                   integerSchema(),
			"name":                 stringSchema(),
			"group_name":           stringSchema(),
			"status":               integerSchema(),
			"reason":               stringSchema(),
			"last_run_time":        dateTimeSchema(),
			"last_failure_time":    dateTimeSchema(),
			"last_failure_message": stringSchema(),
		}, []string{"id", "name", "status", "reason"}),
		"JobHealthCheck": objectSchema(map[string]Schema{
			"total":         integerSchema(),
			"enabled":       integerSchema(),
			"paused":        integerSchema(),
			"recent_failed": integerSchema(),
			"last_run_time": dateTimeSchema(),
			"abnormal_jobs": arraySchema(refSchema("JobAbnormalStatus")),
			"window_hours":  integerSchema(),
			"checked_at":    dateTimeSchema(),
		}, []string{"total", "enabled", "paused", "recent_failed", "abnormal_jobs", "window_hours", "checked_at"}),
		"JobHeartbeat": objectSchema(map[string]Schema{
			"id":               integerSchema(),
			"job_key":          stringSchema(),
			"service":          stringSchema(),
			"description":      stringSchema(),
			"interval_sec":     integerSchema(),
			"last_run_at":      dateTimeSchema(),
			"last_status":      stringSchema(),
			"last_error":       stringSchema(),
			"last_duration_ms": integerSchema(),
			"runs":             integerSchema(),
			"fails":            integerSchema(),
			"updated_at":       dateTimeSchema(),
			"stale":            booleanSchema(),
		}, []string{
			"id", "job_key", "service", "description", "interval_sec", "last_run_at",
			"last_status", "last_error", "last_duration_ms", "runs", "fails", "updated_at", "stale",
		}),
		"JobHeartbeatsResponse": objectSchema(map[string]Schema{
			"list":  arraySchema(refSchema("JobHeartbeat")),
			"total": integerSchema(),
		}, []string{"list", "total"}),
		"ServiceHealthRow": objectSchema(map[string]Schema{
			"name":       stringSchema(),
			"ok":         booleanSchema(),
			"http_code":  integerSchema(),
			"latency_ms": integerSchema(),
			"error":      stringSchema(),
		}, []string{"name", "ok", "http_code", "latency_ms"}),
		"ServicesHealthResponse": objectSchema(map[string]Schema{
			"list":       arraySchema(refSchema("ServiceHealthRow")),
			"total":      integerSchema(),
			"healthy":    integerSchema(),
			"checked_at": dateTimeSchema(),
		}, []string{"list", "total", "healthy", "checked_at"}),
		"ServerOSInfo": objectSchema(map[string]Schema{
			"go_os":         stringSchema(),
			"arch":          stringSchema(),
			"compiler":      stringSchema(),
			"go_version":    stringSchema(),
			"num_goroutine": integerSchema(),
			"hostname":      stringSchema(),
			"platform":      stringSchema(),
			"boot_time":     stringSchema(),
		}, []string{"go_os", "arch", "compiler", "go_version", "num_goroutine", "hostname", "platform", "boot_time"}),
		"ServerCPUInfo": objectSchema(map[string]Schema{
			"model_name":   stringSchema(),
			"cores":        integerSchema(),
			"used_percent": numberSchema(),
		}, []string{"model_name", "cores", "used_percent"}),
		"ServerMemoryInfo": objectSchema(map[string]Schema{
			"total":        integerSchema(),
			"used":         integerSchema(),
			"free":         integerSchema(),
			"used_percent": numberSchema(),
		}, []string{"total", "used", "free", "used_percent"}),
		"ServerDiskInfo": objectSchema(map[string]Schema{
			"total":        integerSchema(),
			"used":         integerSchema(),
			"free":         integerSchema(),
			"used_percent": numberSchema(),
		}, []string{"total", "used", "free", "used_percent"}),
		"ServerInfo": objectSchema(map[string]Schema{
			"os":      refSchema("ServerOSInfo"),
			"runtime": refSchema("ServerOSInfo"),
			"cpu":     refSchema("ServerCPUInfo"),
			"memory":  refSchema("ServerMemoryInfo"),
			"disk":    refSchema("ServerDiskInfo"),
		}, []string{"os", "cpu", "memory", "disk"}),
		"MySQLDatabaseInfo": objectSchema(map[string]Schema{
			"host":        stringSchema(),
			"port":        integerSchema(),
			"name":        stringSchema(),
			"charset":     stringSchema(),
			"collation":   stringSchema(),
			"table_count": integerSchema(),
			"size_bytes":  integerSchema(),
			"size":        stringSchema(),
		}, []string{"host", "port", "name", "table_count", "size_bytes", "size"}),
		"MySQLConnectionInfo": objectSchema(map[string]Schema{
			"max_open_conns":       integerSchema(),
			"open_conns":           integerSchema(),
			"in_use":               integerSchema(),
			"idle":                 integerSchema(),
			"wait_count":           integerSchema(),
			"wait_duration":        stringSchema(),
			"threads_connected":    integerSchema(),
			"threads_running":      integerSchema(),
			"max_connections":      integerSchema(),
			"max_used_connections": integerSchema(),
			"total_connections":    integerSchema(),
		}, []string{"max_open_conns", "open_conns", "in_use", "idle"}),
		"MySQLQueryInfo": objectSchema(map[string]Schema{
			"questions":    integerSchema(),
			"qps":          numberSchema(),
			"slow_queries": integerSchema(),
			"selects":      integerSchema(),
			"inserts":      integerSchema(),
			"updates":      integerSchema(),
			"deletes":      integerSchema(),
		}, []string{"questions", "qps", "slow_queries"}),
		"MySQLTrafficInfo": objectSchema(map[string]Schema{
			"bytes_received":       integerSchema(),
			"bytes_sent":           integerSchema(),
			"bytes_received_human": stringSchema(),
			"bytes_sent_human":     stringSchema(),
		}, []string{"bytes_received", "bytes_sent", "bytes_received_human", "bytes_sent_human"}),
		"MySQLInfo": objectSchema(map[string]Schema{
			"status":         stringSchema(),
			"version":        stringSchema(),
			"uptime":         stringSchema(),
			"uptime_seconds": integerSchema(),
			"database":       refSchema("MySQLDatabaseInfo"),
			"connections":    refSchema("MySQLConnectionInfo"),
			"queries":        refSchema("MySQLQueryInfo"),
			"traffic":        refSchema("MySQLTrafficInfo"),
		}, []string{"status", "version", "uptime", "uptime_seconds", "connections"}),
		"RedisServerInfo": objectSchema(map[string]Schema{
			"version":        stringSchema(),
			"os":             stringSchema(),
			"mode":           stringSchema(),
			"uptime":         stringSchema(),
			"uptime_seconds": integerSchema(),
			"arch_bits":      stringSchema(),
			"process_id":     integerSchema(),
			"tcp_port":       integerSchema(),
		}, []string{"version", "os", "mode", "uptime", "uptime_seconds"}),
		"RedisMemoryInfo": objectSchema(map[string]Schema{
			"used":          stringSchema(),
			"peak":          stringSchema(),
			"lua":           stringSchema(),
			"fragmentation": stringSchema(),
			"used_bytes":    integerSchema(),
			"peak_bytes":    integerSchema(),
			"rss":           stringSchema(),
			"maxmemory":     stringSchema(),
			"mem_allocator": stringSchema(),
			"dataset":       stringSchema(),
			"overhead":      stringSchema(),
		}, []string{"used", "peak", "lua", "fragmentation", "used_bytes", "peak_bytes"}),
		"RedisStatsInfo": objectSchema(map[string]Schema{
			"connections":                stringSchema(),
			"ops":                        stringSchema(),
			"keys":                       integerSchema(),
			"hit_rate":                   stringSchema(),
			"total_connections_received": integerSchema(),
			"total_commands_processed":   integerSchema(),
			"keyspace_hits":              integerSchema(),
			"keyspace_misses":            integerSchema(),
			"expired_keys":               integerSchema(),
			"evicted_keys":               integerSchema(),
		}, []string{"connections", "ops", "keys", "hit_rate"}),
		"RedisClientsInfo": objectSchema(map[string]Schema{
			"connected": integerSchema(),
			"blocked":   integerSchema(),
			"tracking":  integerSchema(),
		}, []string{"connected", "blocked", "tracking"}),
		"RedisPoolInfo": objectSchema(map[string]Schema{
			"hits":        integerSchema(),
			"misses":      integerSchema(),
			"timeouts":    integerSchema(),
			"total_conns": integerSchema(),
			"idle_conns":  integerSchema(),
			"stale_conns": integerSchema(),
		}, []string{"hits", "misses", "timeouts", "total_conns", "idle_conns", "stale_conns"}),
		"RedisKeyspaceInfo": objectSchema(map[string]Schema{
			"dbsize": integerSchema(),
			"dbs":    mapOfSchema(mapOfSchema(integerSchema())),
		}, []string{"dbsize", "dbs"}),
		"RedisInfo": objectSchema(map[string]Schema{
			"status":   stringSchema(),
			"server":   refSchema("RedisServerInfo"),
			"memory":   refSchema("RedisMemoryInfo"),
			"stats":    refSchema("RedisStatsInfo"),
			"clients":  refSchema("RedisClientsInfo"),
			"pool":     refSchema("RedisPoolInfo"),
			"keyspace": refSchema("RedisKeyspaceInfo"),
		}, []string{"status", "server", "memory", "stats", "keyspace"}),
	}

	for envelopeName, schemaName := range map[string]string{
		"EmptyEnvelope":               "EmptyResponse",
		"AlertMetricListEnvelope":     "AlertMetricList",
		"AlertSummaryEnvelope":        "MonitorAlertSummary",
		"AlertRuleEnvelope":           "MonitorAlertRule",
		"AlertRuleListEnvelope":       "AlertRuleListResponse",
		"AlertEventListEnvelope":      "AlertEventListResponse",
		"AlertEvaluationEnvelope":     "AlertEvaluationResult",
		"JobEnvelope":                 "ScheduledJob",
		"JobListEnvelope":             "JobListResponse",
		"JobHealthEnvelope":           "JobHealthCheck",
		"JobHeartbeatsEnvelope":       "JobHeartbeatsResponse",
		"JobLogCleanupResultEnvelope": "JobLogCleanupResult",
		"ServicesHealthEnvelope":      "ServicesHealthResponse",
		"ServerInfoEnvelope":          "ServerInfo",
		"MySQLInfoEnvelope":           "MySQLInfo",
		"RedisInfoEnvelope":           "RedisInfo",
	} {
		schemas[envelopeName] = envelopeFor(refSchema(schemaName))
	}

	return schemas
}

func contractFor(method, path string) (routeContract, bool) {
	contracts := map[string]routeContract{
		"GET /api/v1/monitor/server":                     {ResponseSchema: "ServerInfoEnvelope"},
		"GET /api/v1/monitor/mysql":                      {ResponseSchema: "MySQLInfoEnvelope", UnavailableResponse: true},
		"GET /api/v1/monitor/redis":                      {ResponseSchema: "RedisInfoEnvelope"},
		"GET /api/v1/monitor/alert-metrics":              {ResponseSchema: "AlertMetricListEnvelope"},
		"GET /api/v1/monitor/alert-summary":              {ResponseSchema: "AlertSummaryEnvelope"},
		"GET /api/v1/monitor/alert-rules":                {ResponseSchema: "AlertRuleListEnvelope", QueryParams: alertRuleQueryParams()},
		"POST /api/v1/monitor/alert-rules":               {RequestSchema: "SaveAlertRuleRequest", ResponseSchema: "AlertRuleEnvelope"},
		"PUT /api/v1/monitor/alert-rules/{id}":           {RequestSchema: "SaveAlertRuleRequest", ResponseSchema: "AlertRuleEnvelope", NotFoundResponse: true},
		"DELETE /api/v1/monitor/alert-rules/{id}":        {ResponseSchema: "EmptyEnvelope", NotFoundResponse: true},
		"POST /api/v1/monitor/alert-rules/{id}/evaluate": {ResponseSchema: "AlertEvaluationEnvelope", NoRequestBody: true, UnavailableResponse: true, NotFoundResponse: true},
		"GET /api/v1/monitor/alert-events":               {ResponseSchema: "AlertEventListEnvelope", QueryParams: alertEventQueryParams()},
		"GET /api/v1/monitor/jobs":                       {ResponseSchema: "JobListEnvelope", QueryParams: pagingQueryParams("name", "status"), UnavailableResponse: true},
		"GET /api/v1/monitor/jobs/health":                {ResponseSchema: "JobHealthEnvelope", QueryParams: []Parameter{queryParam("window_hours", minimumIntegerSchema(1))}, UnavailableResponse: true},
		"GET /api/v1/monitor/jobs/heartbeats":            {ResponseSchema: "JobHeartbeatsEnvelope", UnavailableResponse: true},
		"GET /api/v1/monitor/services":                   {ResponseSchema: "ServicesHealthEnvelope"},
		"POST /api/v1/monitor/jobs":                      {RequestSchema: "SaveJobRequest", ResponseSchema: "JobEnvelope", UnavailableResponse: true},
		"PUT /api/v1/monitor/jobs/{id}":                  {RequestSchema: "SaveJobRequest", ResponseSchema: "JobEnvelope", UnavailableResponse: true, NotFoundResponse: true},
		"DELETE /api/v1/monitor/jobs/{id}":               {ResponseSchema: "EmptyEnvelope", UnavailableResponse: true, NotFoundResponse: true},
		"POST /api/v1/monitor/jobs/{id}/start":           {ResponseSchema: "EmptyEnvelope", NoRequestBody: true, UnavailableResponse: true, NotFoundResponse: true},
		"POST /api/v1/monitor/jobs/{id}/stop":            {ResponseSchema: "EmptyEnvelope", NoRequestBody: true, UnavailableResponse: true, NotFoundResponse: true},
		"POST /api/v1/monitor/jobs/{id}/run":             {ResponseSchema: "EmptyEnvelope", NoRequestBody: true, UnavailableResponse: true, NotFoundResponse: true},
		"POST /api/v1/monitor/job-logs/cleanup":          {RequestSchema: "JobLogCleanupRequest", ResponseSchema: "JobLogCleanupResultEnvelope", QueryParams: []Parameter{queryParam("retention_days", minimumIntegerSchema(1))}, OptionalRequestBody: true, UnavailableResponse: true},
	}
	contract, ok := contracts[method+" "+path]
	return contract, ok
}

func envelopeFor(data Schema) Schema {
	return objectSchema(map[string]Schema{
		"code":    integerSchema(),
		"message": stringSchema(),
		"data":    data,
	}, []string{"code", "message", "data"})
}

func objectSchema(properties map[string]Schema, required []string) Schema {
	return Schema{Type: "object", Properties: properties, Required: required}
}

func mapOfSchema(value Schema) Schema {
	return Schema{Type: "object", AdditionalProperties: value}
}

func pageSchema(item Schema) Schema {
	return objectSchema(map[string]Schema{
		"list":      arraySchema(item),
		"total":     integerSchema(),
		"page":      integerSchema(),
		"page_size": integerSchema(),
	}, []string{"list", "total", "page", "page_size"})
}

func refSchema(name string) Schema {
	return Schema{Ref: "#/components/schemas/" + name}
}

func stringSchema() Schema {
	return Schema{Type: "string"}
}

func dateTimeSchema() Schema {
	return Schema{Type: "string", Format: "date-time"}
}

func integerSchema() Schema {
	return Schema{Type: "integer", Format: "int64"}
}

func integerEnumSchema(values ...int) Schema {
	return Schema{Type: "integer", Format: "int64", Enum: values}
}

func minimumIntegerSchema(minimum int) Schema {
	return Schema{Type: "integer", Format: "int64", Minimum: &minimum}
}

func numberSchema() Schema {
	return Schema{Type: "number", Format: "double"}
}

func booleanSchema() Schema {
	return Schema{Type: "boolean"}
}

func arraySchema(item Schema) Schema {
	return Schema{Type: "array", Items: &item}
}

func queryParam(name string, schema Schema) Parameter {
	return Parameter{Name: name, In: "query", Required: false, Schema: schema}
}

func pagingQueryParams(names ...string) []Parameter {
	params := []Parameter{
		queryParam("page", integerSchema()),
		queryParam("page_size", integerSchema()),
	}
	for _, name := range names {
		schema := stringSchema()
		if name == "status" {
			schema = integerEnumSchema(0, 1)
		}
		params = append(params, queryParam(name, schema))
	}
	return params
}

func alertRuleQueryParams() []Parameter {
	return []Parameter{
		queryParam("page", integerSchema()),
		queryParam("page_size", integerSchema()),
		queryParam("name", stringSchema()),
		queryParam("metric", stringSchema()),
		queryParam("state", stringSchema()),
		queryParam("enabled", booleanSchema()),
	}
}

func alertEventQueryParams() []Parameter {
	return []Parameter{
		queryParam("page", integerSchema()),
		queryParam("page_size", integerSchema()),
		queryParam("rule_id", minimumIntegerSchema(1)),
		queryParam("rule_name", stringSchema()),
		queryParam("status", stringSchema()),
		queryParam("severity", stringSchema()),
		queryParam("notify_status", stringSchema()),
	}
}
