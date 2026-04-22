package service

// WindsurfErrorMapper will translate upstream Windsurf failures into the
// project's existing OpenAI/Anthropic-compatible error shapes.
type WindsurfErrorMapper struct{}

func NewWindsurfErrorMapper() *WindsurfErrorMapper {
	return &WindsurfErrorMapper{}
}
