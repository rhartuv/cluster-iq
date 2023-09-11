package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/RHEcosystemAppEng/cluster-iq/internal/inventory"
	ciqLogger "github.com/RHEcosystemAppEng/cluster-iq/internal/logger"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var (
	// version reflects the current version of the API
	version string
	// commit reflects the git short-hash of the compiled version
	commit string
	// TODO: comment rest of global vars
	inven    *inventory.Inventory
	router   *gin.Engine
	apiURL   string
	dbURL    string
	dbPass   string
	rdb      *redis.Client
	ctx      context.Context
	redisKey = "Stock"
	logger   *zap.Logger
)

// InstanceListResponse represents the API response containing a list of clusters
type InstanceListResponse struct {
	Count     int                  `json:"count"`
	Instances []inventory.Instance `json:"instances"`
}

// NewInstanceListResponse creates a new InstanceListResponse instance and
// controls if there is any Instance in the incoming list
func NewInstanceListResponse(instances []inventory.Instance) *InstanceListResponse {
	numInstances := len(instances)

	// If there is no clusters, an empty array is returned instead of null
	if numInstances == 0 {
		instances = []inventory.Instance{}
	}

	response := InstanceListResponse{
		Count:     numInstances,
		Instances: instances,
	}

	return &response
}

// ClusterListResponse represents the API response containing a list of clusters
type ClusterListResponse struct {
	Count    int                 `json:"count"`
	Clusters []inventory.Cluster `json:"clusters"`
}

// NewClusterListResponse creates a new ClusterListResponse instance and
// controls if there is any cluster in the incoming list
func NewClusterListResponse(clusters []inventory.Cluster) *ClusterListResponse {
	numClusters := len(clusters)

	// If there is no clusters, an empty array is returned instead of null
	if numClusters == 0 {
		clusters = []inventory.Cluster{}
	}

	response := ClusterListResponse{
		Count:    numClusters,
		Clusters: clusters,
	}

	return &response
}

// AccountListResponse represents the API response containing a list of accounts
type AccountListResponse struct {
	Count    int                 `json:"count"`
	Accounts []inventory.Account `json:"accounts"`
}

// NewAccountListResponse creates a new ClusterListResponse instance and
// controls if there is any cluster in the incoming list
func NewAccountListResponse(accounts []inventory.Account) *AccountListResponse {
	numAccounts := len(accounts)

	// If there is no clusters, an empty array is returned instead of null
	if numAccounts == 0 {
		accounts = []inventory.Account{}
	}

	response := AccountListResponse{
		Count:    numAccounts,
		Accounts: accounts,
	}

	return &response
}

func init() {
	// Logging config
	logger = ciqLogger.NewLogger()

	// Getting config
	apiHost := os.Getenv("CIQ_API_HOST")
	apiPort := os.Getenv("CIQ_API_PORT")
	dbHost := os.Getenv("CIQ_DB_HOST")
	dbPort := os.Getenv("CIQ_DB_PORT")
	dbPass = os.Getenv("CIQ_DB_PASS")
	apiURL = fmt.Sprintf("%s:%s", apiHost, apiPort)
	dbURL = fmt.Sprintf("%s:%s", dbHost, dbPort)

	// Initializaion global vars
	inven = inventory.NewInventory()
	gin.SetMode(gin.ReleaseMode)
	router = gin.New()

	// Configure GIN to use ZAP
	router.Use(ginzap.Ginzap(logger, time.RFC3339, true))
}

func addHeaders(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
}

// getAccounts returns every account in Stock
func getInstances(c *gin.Context) {
	logger.Debug("Retrieving complete instance inventory")
	updateStock()
	addHeaders(c)

	var instances []inventory.Instance
	for _, account := range inven.Accounts {
		for _, cluster := range account.Clusters {
			for _, instance := range cluster.Instances {
				instances = append(instances, instance)
			}
		}
	}

	response := NewInstanceListResponse(instances)
	c.PureJSON(http.StatusOK, response)
}

// getClusters returns every cluster in Stock
func getClusters(c *gin.Context) {
	logger.Debug("Retrieving complete cluster inventory")
	updateStock()
	addHeaders(c)

	var clusters []inventory.Cluster
	for _, account := range inven.Accounts {
		for _, cluster := range account.Clusters {
			clusters = append(clusters, *cluster)
		}
	}

	response := NewClusterListResponse(clusters)
	c.PureJSON(http.StatusOK, response)
}

// getClusters returns the clusters in Stock with the requested name
func getClustersByName(c *gin.Context) {
	name := c.Param("name")
	logger.Debug("Retrieving clusters by name", zap.String("clusterName", name))
	updateStock()
	addHeaders(c)

	var clusters []inventory.Cluster
	for _, account := range inven.Accounts {
		for _, cluster := range account.Clusters {
			if cluster.Name == name {
				clusters = append(clusters, *cluster)
			}
		}
	}

	response := NewClusterListResponse(clusters)
	c.PureJSON(http.StatusOK, response)
}

// getAccounts returns every account in Stock
func getAccounts(c *gin.Context) {
	logger.Debug("Retrieving complete accounts inventory")
	updateStock()
	addHeaders(c)

	var accounts []inventory.Account

	for _, account := range inven.Accounts {
		accounts = append(accounts, account)
	}

	response := NewAccountListResponse(accounts)
	c.PureJSON(http.StatusOK, response)
}

// getAccountsByName returns an account by its name in Stock
func getAccountsByName(c *gin.Context) {
	name := c.Param("name")
	logger.Debug("Retrieving accounts by name", zap.String("accountName", name))
	updateStock()
	addHeaders(c)

	account, ok := inven.Accounts[name]
	if ok {
		c.PureJSON(http.StatusOK, account)
	} else {
		c.Status(http.StatusNotFound)
	}
}

// updateStock updates the cache of the API
func updateStock() {
	// Getting Redis Results
	val, err := rdb.Get(ctx, redisKey).Result()
	if err != nil {
		logger.Error("Can't connect to DB for inventory updating", zap.Error(err))
	}

	// Unmarshall from JSON to inventory.Inventory type
	json.Unmarshal([]byte(val), &inven)
}

func main() {
	// Ignore Logger sync error
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting ClusterIQ API", zap.String("version", version), zap.String("commit", commit))
	logger.Info("Connection properties", zap.String("api_url", apiURL), zap.String("db_url", dbURL))

	// Preparing API Endpoints
	router.GET("/accounts", getAccounts)
	router.GET("/accounts/:name", getAccountsByName)
	router.GET("/clusters", getClusters)
	router.GET("/clusters/:name", getClustersByName)
	router.GET("/instances", getInstances)
	// Mocked endpoints
	router.GET("/mockedClusters", getMockClusters)
	router.GET("/mockedAccounts", getMockAccounts)

	// RedisDB connection
	ctx = context.Background()
	rdb = redis.NewClient(&redis.Options{
		Addr:     dbURL,
		Password: dbPass,
		DB:       0,
	})

	// Start API
	logger.Info("API Ready to serve")
	router.Run(apiURL)
}
