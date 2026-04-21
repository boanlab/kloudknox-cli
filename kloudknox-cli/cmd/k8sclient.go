// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

type kubeClients struct {
	Dynamic dynamic.Interface
	Typed   *kubernetes.Clientset
	Mapper  meta.RESTMapper
}

// buildKubeClients resolves a REST config from --kubeconfig / --kube-context
// (or the in-cluster service account) and returns all three clients callers
// typically need together.
func buildKubeClients() (*kubeClients, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if globalCfg.KubeConfig != "" {
		loadingRules.ExplicitPath = globalCfg.KubeConfig
	}

	overrides := &clientcmd.ConfigOverrides{}
	if globalCfg.KubeContext != "" {
		overrides.CurrentContext = globalCfg.KubeContext
	}

	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, overrides,
	).ClientConfig()
	if err != nil {
		return nil, err
	}

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}

	typedClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}

	disc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disc))

	return &kubeClients{
		Dynamic: dynClient,
		Typed:   typedClient,
		Mapper:  mapper,
	}, nil
}
