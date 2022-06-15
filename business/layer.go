package business

import (
	"sync"

	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/jaeger"
	"github.com/kiali/kiali/kubernetes"
	"github.com/kiali/kiali/kubernetes/cache"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/prometheus"
)

// Layer is a container for fast access to inner services
type Layer struct {
	Svc            SvcService
	Health         HealthService
	Validations    IstioValidationsService
	IstioConfig    IstioConfigService
	Workload       WorkloadService
	App            AppService
	Namespace      NamespaceService
	Jaeger         JaegerService
	k8s            kubernetes.IstioClientInterface
	OpenshiftOAuth OpenshiftOAuthService
	TLS            TLSService
	ThreeScale     ThreeScaleService
	Iter8          Iter8Service
	IstioStatus    IstioStatusService
}

// Global clientfactory and prometheus clients.
var clientFactory kubernetes.ClientFactory
var prometheusClient prometheus.ClientInterface
var once sync.Once
var kialiCache cache.KialiCache

func initKialiCache() {
	if config.Get().KubernetesConfig.CacheEnabled {
		if cache, err := cache.NewKialiCache(); err != nil {
			log.Errorf("Error initializing Kiali Cache. Details: %s", err)
		} else {
			kialiCache = cache
		}
	}
	if excludedWorkloads == nil {
		excludedWorkloads = make(map[string]bool)
		for _, w := range config.Get().KubernetesConfig.ExcludeWorkloads {
			excludedWorkloads[w] = true
		}
	}
}

func GetUnauthenticated() (*Layer, error) {
	return Get("")
}

// Get the business.Layer
func Get(token string) (*Layer, error) {
	// Kiali Cache will be initialized once at first use of Business layer
	// 只执行一次 . 初始化kiali的缓存：缓存了k8s、istio资源和namespace等信息
	once.Do(initKialiCache)

	// Use an existing client factory if it exists, otherwise create and use in the future
	if clientFactory == nil {
		userClient, err := kubernetes.GetClientFactory()
		if err != nil {
			return nil, err
		}
		clientFactory = userClient
	}

	// Creates a new k8s client based on the current users token
	k8s, err := clientFactory.GetClient(token)
	if err != nil {
		return nil, err
	}

	// Use an existing Prometheus client if it exists, otherwise create and use in the future
	if prometheusClient == nil {
		prom, err := prometheus.NewClient()
		if err != nil {
			return nil, err
		}
		prometheusClient = prom
	}

	// Create Jaeger client
	jaegerLoader := func() (jaeger.ClientInterface, error) {
		return jaeger.NewClient(token)
	}

	return NewWithBackends(k8s, prometheusClient, jaegerLoader), nil
}

// Get the business.Layer NoAuth
func GetNoAuth() (*Layer, error) {
	// Kiali Cache will be initialized once at first use of Business layer
	once.Do(initKialiCache)

	// Use an existing client factory if it exists, otherwise create and use in the future
	if clientFactory == nil {
		userClient, err := kubernetes.GetClientFactoryNoAuth()
		if err != nil {
			return nil, err
		}
		clientFactory = userClient
	}

	// Creates a new k8s client based on the current users token
	k8s, err := clientFactory.GetClientNoAuth()
	if err != nil {
		return nil, err
	}

	// Use an existing Prometheus client if it exists, otherwise create and use in the future
	if prometheusClient == nil {
		prom, err := prometheus.NewClientNoAuth()
		if err != nil {
			return nil, err
		}
		prometheusClient = prom
	}

	// Create Jaeger client
	jaegerLoader := func() (jaeger.ClientInterface, error) {
		return jaeger.NewClientNoAuth()
	}

	return NewWithBackends(k8s, prometheusClient, jaegerLoader), nil
}

// SetWithBackends allows for specifying the ClientFactory and Prometheus clients to be used.
// Mock friendly. Used only with tests.
func SetWithBackends(cf kubernetes.ClientFactory, prom prometheus.ClientInterface) {
	clientFactory = cf
	prometheusClient = prom
}

// NewWithBackends creates the business layer using the passed k8s and prom clients
func NewWithBackends(k8s kubernetes.IstioClientInterface, prom prometheus.ClientInterface, jaegerClient JaegerLoader) *Layer {
	temporaryLayer := &Layer{}
	temporaryLayer.Health = HealthService{prom: prom, k8s: k8s, businessLayer: temporaryLayer}
	temporaryLayer.Svc = SvcService{prom: prom, k8s: k8s, businessLayer: temporaryLayer}
	temporaryLayer.IstioConfig = IstioConfigService{k8s: k8s, businessLayer: temporaryLayer}
	temporaryLayer.Workload = WorkloadService{k8s: k8s, prom: prom, businessLayer: temporaryLayer}
	temporaryLayer.Validations = IstioValidationsService{k8s: k8s, businessLayer: temporaryLayer}
	temporaryLayer.App = AppService{prom: prom, k8s: k8s, businessLayer: temporaryLayer}
	temporaryLayer.Namespace = NewNamespaceService(k8s)
	temporaryLayer.Jaeger = JaegerService{loader: jaegerClient, businessLayer: temporaryLayer}
	temporaryLayer.k8s = k8s
	temporaryLayer.OpenshiftOAuth = OpenshiftOAuthService{k8s: k8s}
	temporaryLayer.TLS = TLSService{k8s: k8s, businessLayer: temporaryLayer}
	temporaryLayer.ThreeScale = ThreeScaleService{k8s: k8s}
	temporaryLayer.Iter8 = Iter8Service{k8s: k8s, businessLayer: temporaryLayer}
	temporaryLayer.IstioStatus = IstioStatusService{k8s: k8s}

	return temporaryLayer
}

func Stop() {
	if kialiCache != nil {
		kialiCache.Stop()
	}
}
