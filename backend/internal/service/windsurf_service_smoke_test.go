package service_test

import (
	"testing"

	handlerpkg "github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestWindsurfServiceSmoke(t *testing.T) {
	probe := service.NewWindsurfAccountProbeService(nil, nil, nil)
	catalog := service.NewWindsurfModelCatalogService()
	usage := service.NewWindsurfUsageFetcher(nil)
	mapper := service.NewWindsurfErrorMapper()
	gateway := service.NewWindsurfGatewayService(probe, catalog, usage, mapper)
	handler := handlerpkg.NewWindsurfGatewayHandler(gateway)

	if probe == nil || catalog == nil || usage == nil || mapper == nil {
		t.Fatal("expected windsurf support services to be constructed")
	}
	if gateway == nil {
		t.Fatal("expected windsurf gateway service to be constructed")
	}
	if gateway.AccountProbe() == nil || gateway.ModelCatalog() == nil || gateway.UsageFetcher() == nil || gateway.ErrorMapper() == nil {
		t.Fatal("expected windsurf gateway service dependencies to be wired")
	}
	if handler == nil {
		t.Fatal("expected windsurf gateway handler to be constructed")
	}
}
