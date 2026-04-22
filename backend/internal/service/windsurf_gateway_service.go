package service

// WindsurfGatewayService is a Phase 1 placeholder used to reserve the Windsurf
// gateway dependency edge before the real upstream proxy logic lands.
type WindsurfGatewayService struct{}

func NewWindsurfGatewayService() *WindsurfGatewayService {
	return &WindsurfGatewayService{}
}
