package events

import (
	"fmt"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/eljuanchosf/firehose-to-syslog/caching"
	"github.com/eljuanchosf/firehose-to-syslog/filters"
	log "github.com/eljuanchosf/firehose-to-syslog/logging"
	"github.com/garyburd/redigo/redis"
)

type Event struct {
	Fields logrus.Fields
	Msg    string
	Type   string
}

var selectedEvents map[string]bool
var redisConn redis.Conn
var redisHash = "LogQuota"

func RouteEvents(in chan *events.Envelope, extraFields map[string]string, filters filters.LogFilters) {
	for msg := range in {
		routeEvent(msg, extraFields, filters)
	}
}

func GetSelectedEvents() map[string]bool {
	return selectedEvents
}

func routeEvent(msg *events.Envelope, extraFields map[string]string, filters filters.LogFilters) {

	eventType := msg.GetEventType()

	if selectedEvents[eventType.String()] {
		var event Event
		switch eventType {
		case events.Envelope_HttpStart:
			event = HttpStart(msg)
		case events.Envelope_HttpStop:
			event = HttpStop(msg)
		case events.Envelope_HttpStartStop:
			event = HttpStartStop(msg)
		case events.Envelope_LogMessage:
			event = LogMessage(msg)
		case events.Envelope_ValueMetric:
			event = ValueMetric(msg)
		case events.Envelope_CounterEvent:
			event = CounterEvent(msg)
		case events.Envelope_Error:
			event = ErrorEvent(msg)
		case events.Envelope_ContainerMetric:
			event = ContainerMetric(msg)
		}

		event.AnnotateWithAppData()
		event.AnnotateWithMetaData(extraFields)

		if event.ApplyFilters(filters) {
			// Compose the key to be used in Redis
			orgId := event.Fields["cf_org_id"].(string)
			orgEnabledKey := orgId + ":enabled"

			// Get the "enabled" flag to know if this organization will keep sending logs to ELK
			orgEnabled, _ := redis.Bool(redisConn.Do("HGET", redisHash, orgEnabledKey))
			if orgEnabled {
				// Increment the value of the organization key to add the amount of bytes of the log
				value, err := redisConn.Do("HINCRBY", redisHash, orgId, len(event.Msg))
				if err != nil || value == nil {
					panic("Redis server could not be contacted!")
				}
				// Send the event (log message)
				event.ShipEvent()
			}
		}
	}
}

func (e *Event) ApplyFilters(filters filters.LogFilters) bool {
	ship := e.ApplyFilter(filters.Orgs)
	ship = ship && e.ApplyFilter(filters.Spaces)
	ship = ship && e.ApplyFilter(filters.Apps)
	return ship
}

func (e *Event) ApplyFilter(filterDefinition filters.CFEntity) bool {
	ship := true
	cfFieldName := filterDefinition.CFFieldName
	for _, filter := range filterDefinition.Filters {
		switch filter.Type {
		case "include":
			if e.Fields[cfFieldName] == filter.Name {
				ship = true
			}
		case "exclude":
			if e.Fields[cfFieldName] == filter.Name {
				ship = false
			}
		}
	}
	return ship
}

func SetupRedisClient(redisConnection redis.Conn) {
	redisConn = redisConnection
}

func SetupEventRouting(wantedEvents string) error {
	selectedEvents = make(map[string]bool)

	if wantedEvents == "" {
		selectedEvents["LogMessage"] = true
	} else {
		for _, event := range strings.Split(wantedEvents, ",") {
			if isAuthorizedEvent(event) {
				selectedEvents[event] = true
				log.LogStd(fmt.Sprintf("Event Type [%s] is included in the fireshose!", event), false)
			} else {
				return fmt.Errorf("Rejected Event Name [%s] - Valid events: %s", event, GetListAuthorizedEventEvents())
			}
		}
	}
	return nil
}

func isAuthorizedEvent(wantedEvent string) bool {
	for _, authorizeEvent := range events.Envelope_EventType_name {
		if wantedEvent == authorizeEvent {
			return true
		}
	}
	return false
}

func GetListAuthorizedEventEvents() (authorizedEvents string) {
	arrEvents := []string{}
	for _, listEvent := range events.Envelope_EventType_name {
		arrEvents = append(arrEvents, listEvent)
	}
	return strings.Join(arrEvents, ", ")
}

func getAppInfo(appGuid string) caching.App {
	if app := caching.GetAppInfo(appGuid); app.Name != "" {
		return app
	} else {
		caching.GetAppByGuid(appGuid)
	}
	return caching.GetAppInfo(appGuid)
}

func HttpStart(msg *events.Envelope) Event {
	httpStart := msg.GetHttpStart()

	fields := logrus.Fields{
		"origin":            msg.GetOrigin(),
		"cf_app_id":         httpStart.GetApplicationId(),
		"instance_id":       httpStart.GetInstanceId(),
		"instance_index":    httpStart.GetInstanceIndex(),
		"method":            httpStart.GetMethod(),
		"parent_request_id": httpStart.GetParentRequestId(),
		"peer_type":         httpStart.GetPeerType(),
		"request_id":        httpStart.GetRequestId(),
		"remote_addr":       httpStart.GetRemoteAddress(),
		"timestamp":         httpStart.GetTimestamp(),
		"uri":               httpStart.GetUri(),
		"user_agent":        httpStart.GetUserAgent(),
	}

	return Event{
		Fields: fields,
		Msg:    "",
		Type:   msg.GetEventType().String(),
	}
}

func HttpStop(msg *events.Envelope) Event {
	httpStop := msg.GetHttpStop()

	fields := logrus.Fields{
		"origin":         msg.GetOrigin(),
		"cf_app_id":      httpStop.GetApplicationId(),
		"content_length": httpStop.GetContentLength(),
		"peer_type":      httpStop.GetPeerType(),
		"request_id":     httpStop.GetRequestId(),
		"status_code":    httpStop.GetStatusCode(),
		"timestamp":      httpStop.GetTimestamp(),
		"uri":            httpStop.GetUri(),
	}

	return Event{
		Fields: fields,
		Msg:    "",
		Type:   msg.GetEventType().String(),
	}
}

func HttpStartStop(msg *events.Envelope) Event {
	httpStartStop := msg.GetHttpStartStop()

	fields := logrus.Fields{
		"origin":            msg.GetOrigin(),
		"cf_app_id":         httpStartStop.GetApplicationId(),
		"content_length":    httpStartStop.GetContentLength(),
		"instance_id":       httpStartStop.GetInstanceId(),
		"instance_index":    httpStartStop.GetInstanceIndex(),
		"method":            httpStartStop.GetMethod(),
		"parent_request_id": httpStartStop.GetParentRequestId(),
		"peer_type":         httpStartStop.GetPeerType(),
		"remote_addr":       httpStartStop.GetRemoteAddress(),
		"request_id":        httpStartStop.GetRequestId(),
		"start_timestamp":   httpStartStop.GetStartTimestamp(),
		"status_code":       httpStartStop.GetStatusCode(),
		"stop_timestamp":    httpStartStop.GetStopTimestamp(),
		"uri":               httpStartStop.GetUri(),
		"user_agent":        httpStartStop.GetUserAgent(),
		"duration_ms":       (((httpStartStop.GetStopTimestamp() - httpStartStop.GetStartTimestamp()) / 1000) / 1000),
	}

	return Event{
		Fields: fields,
		Msg:    "",
		Type:   msg.GetEventType().String(),
	}
}

func LogMessage(msg *events.Envelope) Event {
	logMessage := msg.GetLogMessage()

	fields := logrus.Fields{
		"origin":          msg.GetOrigin(),
		"cf_app_id":       logMessage.GetAppId(),
		"timestamp":       logMessage.GetTimestamp(),
		"source_type":     logMessage.GetSourceType(),
		"message_type":    logMessage.GetMessageType().String(),
		"source_instance": logMessage.GetSourceInstance(),
	}

	return Event{
		Fields: fields,
		Msg:    string(logMessage.GetMessage()),
		Type:   msg.GetEventType().String(),
	}
}

func ValueMetric(msg *events.Envelope) Event {
	valMetric := msg.GetValueMetric()

	fields := logrus.Fields{
		"origin": msg.GetOrigin(),
		"name":   valMetric.GetName(),
		"unit":   valMetric.GetUnit(),
		"value":  valMetric.GetValue(),
	}

	return Event{
		Fields: fields,
		Msg:    "",
		Type:   msg.GetEventType().String(),
	}
}

func CounterEvent(msg *events.Envelope) Event {
	counterEvent := msg.GetCounterEvent()

	fields := logrus.Fields{
		"origin": msg.GetOrigin(),
		"name":   counterEvent.GetName(),
		"delta":  counterEvent.GetDelta(),
		"total":  counterEvent.GetTotal(),
	}

	return Event{
		Fields: fields,
		Msg:    "",
		Type:   msg.GetEventType().String(),
	}
}

func ErrorEvent(msg *events.Envelope) Event {
	errorEvent := msg.GetError()

	fields := logrus.Fields{
		"origin": msg.GetOrigin(),
		"code":   errorEvent.GetCode(),
		"delta":  errorEvent.GetSource(),
	}

	return Event{
		Fields: fields,
		Msg:    errorEvent.GetMessage(),
		Type:   msg.GetEventType().String(),
	}
}

func ContainerMetric(msg *events.Envelope) Event {
	containerMetric := msg.GetContainerMetric()

	fields := logrus.Fields{
		"origin":         msg.GetOrigin(),
		"cf_app_id":      containerMetric.GetApplicationId(),
		"cpu_percentage": containerMetric.GetCpuPercentage(),
		"disk_bytes":     containerMetric.GetDiskBytes(),
		"instance_index": containerMetric.GetInstanceIndex(),
		"memory_bytes":   containerMetric.GetMemoryBytes(),
	}

	return Event{
		Fields: fields,
		Msg:    "",
		Type:   msg.GetEventType().String(),
	}
}

func (e *Event) AnnotateWithAppData() {

	cf_app_id := e.Fields["cf_app_id"]
	appGuid := ""
	if cf_app_id != nil {
		appGuid = fmt.Sprintf("%s", cf_app_id)
	}

	if cf_app_id != nil && appGuid != "<nil>" && cf_app_id != "" {
		appInfo := getAppInfo(appGuid)
		cf_app_name := appInfo.Name
		cf_space_id := appInfo.SpaceGuid
		cf_space_name := appInfo.SpaceName
		cf_org_id := appInfo.OrgGuid
		cf_org_name := appInfo.OrgName

		if cf_app_name != "" {
			e.Fields["cf_app_name"] = cf_app_name
		}

		if cf_space_id != "" {
			e.Fields["cf_space_id"] = cf_space_id
		}

		if cf_space_name != "" {
			e.Fields["cf_space_name"] = cf_space_name
		}

		if cf_org_id != "" {
			e.Fields["cf_org_id"] = cf_org_id
		}

		if cf_org_name != "" {
			e.Fields["cf_org_name"] = cf_org_name
		}
	}
}

func (e *Event) AnnotateWithMetaData(extraFields map[string]string) {
	e.Fields["cf_origin"] = "firehose"
	e.Fields["event_type"] = e.Type
	for k, v := range extraFields {
		e.Fields[k] = v
	}
}

func (e Event) ShipEvent() {

	defer func() {
		if r := recover(); r != nil {
			log.LogError("Recovered in event.Log()", r)
		}
	}()

	logrus.WithFields(e.Fields).Info(e.Msg)
}
