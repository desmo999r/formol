package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

var SchemeGroupVersion = schema.GroupVersion{Group: "formol.desmojim.fr", Version: "v1alpha1"}

type BackupClient struct {
	restClient rest.Interface
}

func NewClient(conf *rest.Config) (*BackupClient, error){
	scheme := runtime.NewScheme()
	schemeBuilder := runtime.NewSchemeBuilder(func (scheme *runtime.Scheme) error {
		scheme.AddKnowTypes(SchemeGroupVersion,
			&BackupSession{},
			&BackupSessionList{},
		)
		metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	})
	if err := schemeBuilder.AddToScheme(scheme); err != nil {
		panic(err.Error())
	}
	config := *conf
	config.GroupVersion = &SchemeGroupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{
		CodecFactory: serializer.NewCodecFactory(scheme)
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		panic(err.Error())
	}
	return &BackupClient{restClient: client}, nil
}

