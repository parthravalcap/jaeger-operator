package strategy

import (
	"strings"

	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"

	"github.com/jaegertracing/jaeger-operator/pkg/account"
	"github.com/jaegertracing/jaeger-operator/pkg/apis/io/v1alpha1"
	"github.com/jaegertracing/jaeger-operator/pkg/config/sampling"
	"github.com/jaegertracing/jaeger-operator/pkg/config/ui"
	"github.com/jaegertracing/jaeger-operator/pkg/cronjob"
	"github.com/jaegertracing/jaeger-operator/pkg/deployment"
	"github.com/jaegertracing/jaeger-operator/pkg/ingress"
	"github.com/jaegertracing/jaeger-operator/pkg/inject"
	"github.com/jaegertracing/jaeger-operator/pkg/route"
	"github.com/jaegertracing/jaeger-operator/pkg/storage"
)

func newProductionStrategy(jaeger *v1alpha1.Jaeger) S {
	c := S{typ: Production}
	collector := deployment.NewCollector(jaeger)
	query := deployment.NewQuery(jaeger)
	agent := deployment.NewAgent(jaeger)

	// add all service accounts
	for _, acc := range account.Get(jaeger) {
		c.accounts = append(c.accounts, *acc)
	}

	// add the config map
	if cm := configmap.NewUIConfig(jaeger).Get(); cm != nil {
		c.configMaps = append(c.configMaps, *cm)
	}

	// add the Sampling config map
	if cm := sampling.NewConfig(jaeger).Get(); cm != nil {
		c.configMaps = append(c.configMaps, *cm)
	}

	// add the daemonsets
	if ds := agent.Get(); ds != nil {
		c.daemonSets = []appsv1.DaemonSet{*ds}
	}

	// add the services
	for _, svc := range collector.Services() {
		c.services = append(c.services, *svc)
	}

	for _, svc := range query.Services() {
		c.services = append(c.services, *svc)
	}

	// add the routes/ingresses
	if viper.GetString("platform") == v1alpha1.FlagPlatformOpenShift {
		if q := route.NewQueryRoute(jaeger).Get(); nil != q {
			c.routes = append(c.routes, *q)
		}
	} else {
		if q := ingress.NewQueryIngress(jaeger).Get(); nil != q {
			c.ingresses = append(c.ingresses, *q)
		}
	}

	if isBoolTrue(jaeger.Spec.Storage.SparkDependencies.Enabled) {
		if cronjob.SupportedStorage(jaeger.Spec.Storage.Type) {
			c.cronJobs = append(c.cronJobs, *cronjob.CreateSparkDependencies(jaeger))
		} else {
			jaeger.Logger().WithField("type", jaeger.Spec.Storage.Type).Warn("Skipping spark dependencies job due to unsupported storage.")
		}
	}

	var indexCleaner *batchv1beta1.CronJob
	if isBoolTrue(jaeger.Spec.Storage.EsIndexCleaner.Enabled) {
		if strings.EqualFold(jaeger.Spec.Storage.Type, "elasticsearch") {
			indexCleaner = cronjob.CreateEsIndexCleaner(jaeger)
		} else {
			jaeger.Logger().WithField("type", jaeger.Spec.Storage.Type).Warn("Skipping Elasticsearch index cleaner job due to unsupported storage.")
		}
	}

	// prepare the deployments, which may get changed by the elasticsearch routine
	cDep := collector.Get()
	queryDep := inject.OAuthProxy(jaeger, query.Get())

	// assembles the pieces for an elasticsearch self-provisioned deployment via the elasticsearch operator
	if storage.ShouldDeployElasticsearch(jaeger.Spec.Storage) {
		es := &storage.ElasticsearchDeployment{
			Jaeger: jaeger,
		}

		err := storage.CreateESCerts()
		if err != nil {
			jaeger.Logger().WithError(err).Error("failed to create Elasticsearch certificates, Elasticsearch won't be deployed")
		} else {
			c.secrets = storage.ESSecrets(jaeger)
			c.roles = append(c.roles, storage.ESRole(jaeger))
			c.roleBindings = append(
				c.roleBindings,
				storage.ESRoleBinding(jaeger,
					cDep.Spec.Template.Spec.ServiceAccountName,
					queryDep.Spec.Template.Spec.ServiceAccountName,
				),
			)
			c.elasticsearches = append(c.elasticsearches, *es.Elasticsearch())

			es.InjectStorageConfiguration(&queryDep.Spec.Template.Spec)
			es.InjectStorageConfiguration(&cDep.Spec.Template.Spec)
			if indexCleaner != nil {
				es.InjectIndexCleanerConfiguration(&indexCleaner.Spec.JobTemplate.Spec.Template.Spec)
			}
		}
	}

	// the index cleaner ES job, which may have been changed by the ES self-provisioning routine
	if indexCleaner != nil {
		c.cronJobs = append(c.cronJobs, *indexCleaner)
	}

	// add the deployments, which may have been changed by the ES self-provisioning routine
	c.deployments = []appsv1.Deployment{*cDep, *queryDep}

	return c
}