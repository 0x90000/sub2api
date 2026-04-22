package service

// WindsurfGatewayService owns the provider-facing Windsurf request flow.
// Phase 2 starts by wiring a stable internal service graph before the real
// upstream proxy logic is implemented.
type WindsurfGatewayService struct {
	accountProbe *WindsurfAccountProbeService
	modelCatalog *WindsurfModelCatalogService
	usageFetcher *WindsurfUsageFetcher
	errorMapper  *WindsurfErrorMapper
}

func NewWindsurfGatewayService(
	accountProbe *WindsurfAccountProbeService,
	modelCatalog *WindsurfModelCatalogService,
	usageFetcher *WindsurfUsageFetcher,
	errorMapper *WindsurfErrorMapper,
) *WindsurfGatewayService {
	return &WindsurfGatewayService{
		accountProbe: accountProbe,
		modelCatalog: modelCatalog,
		usageFetcher: usageFetcher,
		errorMapper:  errorMapper,
	}
}

func (s *WindsurfGatewayService) AccountProbe() *WindsurfAccountProbeService {
	if s == nil {
		return nil
	}
	return s.accountProbe
}

func (s *WindsurfGatewayService) ModelCatalog() *WindsurfModelCatalogService {
	if s == nil {
		return nil
	}
	return s.modelCatalog
}

func (s *WindsurfGatewayService) UsageFetcher() *WindsurfUsageFetcher {
	if s == nil {
		return nil
	}
	return s.usageFetcher
}

func (s *WindsurfGatewayService) ErrorMapper() *WindsurfErrorMapper {
	if s == nil {
		return nil
	}
	return s.errorMapper
}
