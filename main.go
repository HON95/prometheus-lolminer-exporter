package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const defaultEndpoint = ":8080"
const namespace = "lolminer"

var enableDebug = false
var endpoint = defaultEndpoint
var metricsEndpoint = ""

type lolMinerResult struct {
	Software string                `json:"Software"`
	Mining   lolMinerMiningResult  `json:"Mining"`
	Stratum  lolMinerStratumResult `json:"Stratum"`
	Session  lolMinerSessionResult `json:"Session"`
	GPUs     []lolMinerGPUResult   `json:"GPUs"`
}

type lolMinerMiningResult struct {
	Algorithm string `json:"Algorithm"`
}

type lolMinerStratumResult struct {
	CurrentPool      string  `json:"Current_Pool"`
	CurrentUser      string  `json:"Current_User"`
	AverageLatencyMs float64 `json:"Average_Latency"`
}

type lolMinerSessionResult struct {
	Startup          int64   `json:"Startup"`
	StartupString    string  `json:"Startup_String"`
	Uptime           int64   `json:"Uptime"`
	LastUpdate       int64   `json:"Last_Update"`
	ActiveGPUs       int64   `json:"Active_GPUs"`
	TotalPerformance float64 `json:"Performance_Summary"`
	PerformanceUnit  string  `json:"Performance_Unit"`
	AcceptedShares   int64   `json:"Accepted"`
	SubmittedShares  int64   `json:"Submitted"`
	TotalPower       float64 `json:"TotalPower"`
}

type lolMinerGPUResult struct {
	Index                  int64   `json:"Index"`
	Name                   string  `json:"Name"`
	Performance            float64 `json:"Performance"`
	Power                  float64 `json:"Consumption (W)"`
	FanSpeedPercent        float64 `json:"Fan Speed (%)"`
	Temperature            float64 `json:"Temp (deg C)"`
	MinTemperature         float64 `json:"Mem Temp (deg C)"`
	SessionAcceptedShares  int64   `json:"Session_Accepted"`
	SessionSubmittedShares int64   `json:"Session_Submitted"`
	SessionHWErrors        int64   `json:"Session_HWErr"`
	PCIEAddress            string  `json:"PCIE_Address"`
}

func main() {
	fmt.Printf("%s version %s by %s.\n", appName, appVersion, appAuthor)

	parseCliArgs()
	if enableDebug {
		fmt.Printf("[DEBUG] Debug mode enabled.\n")
	}

	if err := runServer(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return
	}
}

func parseCliArgs() {
	flag.BoolVar(&enableDebug, "debug", false, "Show debug messages.")
	flag.StringVar(&endpoint, "endpoint", defaultEndpoint, "The address-port endpoint to bind to.")

	// Exits on error
	flag.Parse()
}

func runServer() error {
	fmt.Printf("Listening on %s.\n", endpoint)
	var mainServeMux http.ServeMux
	mainServeMux.HandleFunc("/", handleOtherRequest)
	mainServeMux.HandleFunc("/metrics", handleScrapeRequest)
	if err := http.ListenAndServe(endpoint, &mainServeMux); err != nil {
		return fmt.Errorf("Error while running main HTTP server: %s", err)
	}
	return nil
}

func handleOtherRequest(response http.ResponseWriter, request *http.Request) {
	fmt.Fprintf(response, "%s version %s by %s.\n\n", appName, appVersion, appAuthor)
	fmt.Fprintf(response, "Usage: /metrics?target=<target>.\n")
}

func handleScrapeRequest(response http.ResponseWriter, request *http.Request) {
	if enableDebug {
		fmt.Printf("[DEBUG] Request: %s\n", request.RemoteAddr)
	}

	// Get and parse target
	targetURL := parseTargetURL(response, request)
	if targetURL == nil {
		return
	}

	// Scrape target and parse data
	data := scrapeTarget(response, targetURL)
	if data == nil {
		return
	}

	// Build registry with data
	registry := buildRegistry(response, data)
	if registry == nil {
		return
	}

	// Delegare final handling to Prometheus
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	handler.ServeHTTP(response, request)
}

// Returns the target URL if successful and nil if not.
func parseTargetURL(response http.ResponseWriter, request *http.Request) *url.URL {
	var rawTarget string
	if values, ok := request.URL.Query()["target"]; ok && len(values) > 0 && values[0] != "" {
		rawTarget = values[0]
	} else {
		http.Error(response, "400 - Missing target.\n", 400)
		return nil
	}
	if !strings.HasPrefix(rawTarget, "http://") && !strings.HasPrefix(rawTarget, "https://") {
		rawTarget = "http://" + rawTarget
	}
	targetURL, targetURLErr := url.ParseRequestURI(rawTarget)
	if targetURLErr != nil {
		message := fmt.Sprintf("400 - Invalid target: %s\n", targetURLErr)
		http.Error(response, message, 400)
		return nil
	}
	return targetURL
}

// Scrapes the target and returns the parsed data if successful or nil if not.
func scrapeTarget(response http.ResponseWriter, targetURL *url.URL) *lolMinerResult {
	// Scrape
	scrapeRequest, scrapeRequestErr := http.NewRequest("GET", targetURL.String(), nil)
	if scrapeRequestErr != nil {
		if enableDebug {
			fmt.Printf("[DEBUG] Failed to make request to scrape target:\n%v", scrapeRequestErr)
		}
		message := fmt.Sprintf("500 - Failed to scrape target: %s\n", scrapeRequestErr)
		http.Error(response, message, 500)
		return nil
	}
	scrapeClient := http.Client{}
	scrapeResponse, scrapeResponseErr := scrapeClient.Do(scrapeRequest)
	if scrapeResponseErr != nil {
		if enableDebug {
			fmt.Printf("[DEBUG] Failed to scrape target:\n%v", scrapeResponseErr)
		}
		message := fmt.Sprintf("500 - Failed to scrape target: %s\n", scrapeResponseErr)
		http.Error(response, message, 500)
		return nil
	}
	defer scrapeResponse.Body.Close()
	rawData, rawDataErr := ioutil.ReadAll(scrapeResponse.Body)
	if rawDataErr != nil {
		if enableDebug {
			fmt.Printf("[DEBUG] Failed to read data from target:\n%v", rawDataErr)
		}
		message := fmt.Sprintf("500 - Failed to scrape target: %s\n", rawDataErr)
		http.Error(response, message, 500)
		return nil
	}

	// Parse
	var data lolMinerResult
	if err := json.Unmarshal(rawData, &data); err != nil {
		if enableDebug {
			fmt.Printf("[DEBUG] Failed to unmarshal data from target:\n%v", err)
		}
		message := fmt.Sprintf("500 - Failed to parse scraped data: %s\n", err)
		http.Error(response, message, 500)
		return nil
	}

	// Validate
	if data.Session.PerformanceUnit != "mh/s" {
		message := fmt.Sprintf("500 - Target returned unexpected performance unit (expected \"mh/s\"): %s\n", data.Session.PerformanceUnit)
		http.Error(response, message, 500)
		return nil
	}

	return &data
}

// Builds a new registry, adds scraped data to it and returns it if successful or nil if not.
func buildRegistry(response http.ResponseWriter, data *lolMinerResult) *prometheus.Registry {
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGoCollector())

	addExporterMetrics(registry)
	addSoftwareMetrics(registry, data)
	addMiningMetrics(registry, &data.Mining)
	addStratumMetrics(registry, &data.Stratum)
	addSessionMetrics(registry, &data.Session)
	for _, gpuData := range data.GPUs {
		addGPUMetrics(registry, &gpuData)
	}

	return registry
}

func addExporterMetrics(registry *prometheus.Registry) {
	// Info
	infoLabels := make(prometheus.Labels)
	infoLabels["version"] = appVersion
	var infoMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "exporter_info",
		Help:      "Metadata about the exporter.",
	}, labelsKeys(infoLabels))
	infoMetric.With(infoLabels).Set(1)
	registry.MustRegister(infoMetric)
}

func addSoftwareMetrics(registry *prometheus.Registry, data *lolMinerResult) {
	// Info
	infoLabels := make(prometheus.Labels)
	infoLabels["software"] = data.Software
	var infoMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "software_info",
		Help:      "Metadata about the software.",
	}, labelsKeys(infoLabels))
	infoMetric.With(infoLabels).Set(1)
	registry.MustRegister(infoMetric)
}

func addMiningMetrics(registry *prometheus.Registry, data *lolMinerMiningResult) {
	// Info
	infoLabels := make(prometheus.Labels)
	infoLabels["algorithm"] = data.Algorithm
	var infoMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "mining_info",
		Help:      "Metadata about mining.",
	}, labelsKeys(infoLabels))
	infoMetric.With(infoLabels).Set(1)
	registry.MustRegister(infoMetric)
}

func addStratumMetrics(registry *prometheus.Registry, data *lolMinerStratumResult) {
	// Common labels for subsystem
	commonLabels := make(prometheus.Labels)
	commonLabels["stratum_pool"] = data.CurrentPool
	commonLabels["stratum_user"] = data.CurrentUser

	// Info
	infoLabels := make(prometheus.Labels, len(commonLabels))
	for k, v := range commonLabels {
		infoLabels[k] = v
	}
	var infoMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "stratum_info",
		Help:      "Metadata about the stratum.",
	}, labelsKeys(infoLabels))
	infoMetric.With(infoLabels).Set(1)
	registry.MustRegister(infoMetric)

	// Avg. latency
	var avgLatencyMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "stratum_average_latency_seconds",
		Help:      "Average latency for the stratum (s).",
	}, labelsKeys(commonLabels))
	avgLatencyMetric.With(commonLabels).Set(data.AverageLatencyMs / 1000)
	registry.MustRegister(avgLatencyMetric)
}

func addSessionMetrics(registry *prometheus.Registry, data *lolMinerSessionResult) {
	// Common labels for subsystem
	commonLabels := make(prometheus.Labels)
	commonLabels["session_startup_time"] = data.StartupString

	// Info
	infoLabels := make(prometheus.Labels, len(commonLabels))
	for k, v := range commonLabels {
		infoLabels[k] = v
	}
	var infoMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "session_info",
		Help:      "Metadata about the session.",
	}, labelsKeys(infoLabels))
	infoMetric.With(infoLabels).Set(1)
	registry.MustRegister(infoMetric)

	// Startup
	var startupMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "session_startup_seconds_timestamp",
		Help:      "Timestamp for the start of the session.",
	}, labelsKeys(commonLabels))
	startupMetric.With(commonLabels).Set(float64(data.Startup))
	registry.MustRegister(startupMetric)

	// Uptime
	var uptimeMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "session_uptime_seconds",
		Help:      "Uptime for the session (s).",
	}, labelsKeys(commonLabels))
	uptimeMetric.With(commonLabels).Set(float64(data.Uptime))
	registry.MustRegister(uptimeMetric)

	// Last update
	var lastUpdateMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "session_last_update_seconds_timestamp",
		Help:      "Timestamp for last update.",
	}, labelsKeys(commonLabels))
	lastUpdateMetric.With(commonLabels).Set(float64(data.LastUpdate))
	registry.MustRegister(lastUpdateMetric)

	// Active GPUs
	var activeGPUsMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "session_active_gpus",
		Help:      "Number of active GPUs.",
	}, labelsKeys(commonLabels))
	activeGPUsMetric.With(commonLabels).Set(float64(data.ActiveGPUs))
	registry.MustRegister(activeGPUsMetric)

	// Performance
	var totalPerformanceMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "session_performance_total_mhps",
		Help:      "Total current performance for the session (Mh/s).",
	}, labelsKeys(commonLabels))
	totalPerformanceMetric.With(commonLabels).Set(float64(data.TotalPerformance))
	registry.MustRegister(totalPerformanceMetric)

	// Accepted shares
	var acceptedSharesMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "session_accepted_shares_total",
		Help:      "Number of accepted shares for this session.",
	}, labelsKeys(commonLabels))
	acceptedSharesMetric.With(commonLabels).Add(float64(data.AcceptedShares))
	registry.MustRegister(acceptedSharesMetric)

	// Submitted shares
	var submittedSharesMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "session_submitted_shares_total",
		Help:      "Number of submitted shares for this session.",
	}, labelsKeys(commonLabels))
	submittedSharesMetric.With(commonLabels).Add(float64(data.SubmittedShares))
	registry.MustRegister(submittedSharesMetric)

	// Total power
	var totalPowerMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "session_power_total_watts",
		Help:      "Total current power usage for the session (Watt).",
	}, labelsKeys(commonLabels))
	totalPowerMetric.With(commonLabels).Set(float64(data.TotalPower))
	registry.MustRegister(totalPowerMetric)
}

func addGPUMetrics(registry *prometheus.Registry, data *lolMinerGPUResult) {
	// Common labels for subsystem
	commonLabels := make(prometheus.Labels)
	commonLabels["gpu_index"] = fmt.Sprintf("%d", data.Index)

	// Info
	infoLabels := make(prometheus.Labels, len(commonLabels))
	for k, v := range commonLabels {
		infoLabels[k] = v
	}
	infoLabels["name"] = data.Name
	infoLabels["pcie_address"] = data.PCIEAddress
	var infoMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "gpu_info",
		Help:      "Metadata about a GPU.",
	}, labelsKeys(infoLabels))
	infoMetric.With(infoLabels).Set(1)
	registry.MustRegister(infoMetric)

	// Performance
	var performanceMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "gpu_performance_mhps",
		Help:      "GPU performance (Mh/s).",
	}, labelsKeys(commonLabels))
	performanceMetric.With(commonLabels).Set(float64(data.Performance))
	registry.MustRegister(performanceMetric)

	// Power
	var powerMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "gpu_power_watts",
		Help:      "GPU power usage (Watt).",
	}, labelsKeys(commonLabels))
	powerMetric.With(commonLabels).Set(float64(data.Power))
	registry.MustRegister(powerMetric)

	// Fan speed
	var fanSpeedMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "gpu_fan_speed",
		Help:      "GPU fan speed (0-1).",
	}, labelsKeys(commonLabels))
	fanSpeedMetric.With(commonLabels).Set(float64(data.FanSpeedPercent / 100))
	registry.MustRegister(fanSpeedMetric)

	// Temperature
	var temperatureMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "gpu_temperature_celsius",
		Help:      "GPU temperature (deg. C).",
	}, labelsKeys(commonLabels))
	temperatureMetric.With(commonLabels).Set(float64(data.Temperature))
	registry.MustRegister(temperatureMetric)

	// Session accepted shares
	var sessionAcceptedSharesMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "gpu_session_accepted_shares_total",
		Help:      "Number of accepted shared for the GPU during the current session.",
	}, labelsKeys(commonLabels))
	sessionAcceptedSharesMetric.With(commonLabels).Add(float64(data.SessionAcceptedShares))
	registry.MustRegister(sessionAcceptedSharesMetric)

	// Session submitted shares
	var sessionSubmittedSharesMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "gpu_session_submitted_shares_total",
		Help:      "Number of submitted shared for the GPU during the current session.",
	}, labelsKeys(commonLabels))
	sessionSubmittedSharesMetric.With(commonLabels).Add(float64(data.SessionSubmittedShares))
	registry.MustRegister(sessionSubmittedSharesMetric)

	// Session HW errors
	var sessionHwErrorsMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "gpu_session_hardware_errors_total",
		Help:      "Number of hardware errors for the GPU during the current session.",
	}, labelsKeys(commonLabels))
	sessionHwErrorsMetric.With(commonLabels).Add(float64(data.SessionHWErrors))
	registry.MustRegister(sessionHwErrorsMetric)
}

func labelsKeys(fullMap prometheus.Labels) []string {
	keys := make([]string, len(fullMap))
	i := 0
	for key := range fullMap {
		keys[i] = key
		i++
	}
	return keys
}
