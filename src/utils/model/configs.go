package model

type ModelConfig struct {
	FirstParty string
	Bedrock    string
	Vertex     string
	Foundry    string
}

type ModelStrings struct {
	Haiku35  string
	Haiku45  string
	Sonnet35 string
	Sonnet37 string
	Sonnet40 string
	Sonnet45 string
	Sonnet46 string
	Opus40   string
	Opus41   string
	Opus45   string
	Opus46   string
}

var allModelConfigs = struct {
	Haiku35  ModelConfig
	Haiku45  ModelConfig
	Sonnet35 ModelConfig
	Sonnet37 ModelConfig
	Sonnet40 ModelConfig
	Sonnet45 ModelConfig
	Sonnet46 ModelConfig
	Opus40   ModelConfig
	Opus41   ModelConfig
	Opus45   ModelConfig
	Opus46   ModelConfig
}{
	Haiku35: ModelConfig{
		FirstParty: "claude-3-5-haiku-20241022",
		Bedrock:    "us.anthropic.claude-3-5-haiku-20241022-v1:0",
		Vertex:     "claude-3-5-haiku@20241022",
		Foundry:    "claude-3-5-haiku",
	},
	Haiku45: ModelConfig{
		FirstParty: "claude-haiku-4-5-20251001",
		Bedrock:    "us.anthropic.claude-haiku-4-5-20251001-v1:0",
		Vertex:     "claude-haiku-4-5@20251001",
		Foundry:    "claude-haiku-4-5",
	},
	Sonnet35: ModelConfig{
		FirstParty: "claude-3-5-sonnet-20241022",
		Bedrock:    "anthropic.claude-3-5-sonnet-20241022-v2:0",
		Vertex:     "claude-3-5-sonnet-v2@20241022",
		Foundry:    "claude-3-5-sonnet",
	},
	Sonnet37: ModelConfig{
		FirstParty: "claude-3-7-sonnet-20250219",
		Bedrock:    "us.anthropic.claude-3-7-sonnet-20250219-v1:0",
		Vertex:     "claude-3-7-sonnet@20250219",
		Foundry:    "claude-3-7-sonnet",
	},
	Sonnet40: ModelConfig{
		FirstParty: "claude-sonnet-4-20250514",
		Bedrock:    "us.anthropic.claude-sonnet-4-20250514-v1:0",
		Vertex:     "claude-sonnet-4@20250514",
		Foundry:    "claude-sonnet-4",
	},
	Sonnet45: ModelConfig{
		FirstParty: "claude-sonnet-4-5-20250929",
		Bedrock:    "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
		Vertex:     "claude-sonnet-4-5@20250929",
		Foundry:    "claude-sonnet-4-5",
	},
	Sonnet46: ModelConfig{
		FirstParty: "claude-sonnet-4-6",
		Bedrock:    "us.anthropic.claude-sonnet-4-6",
		Vertex:     "claude-sonnet-4-6",
		Foundry:    "claude-sonnet-4-6",
	},
	Opus40: ModelConfig{
		FirstParty: "claude-opus-4-20250514",
		Bedrock:    "us.anthropic.claude-opus-4-20250514-v1:0",
		Vertex:     "claude-opus-4@20250514",
		Foundry:    "claude-opus-4",
	},
	Opus41: ModelConfig{
		FirstParty: "claude-opus-4-1-20250805",
		Bedrock:    "us.anthropic.claude-opus-4-1-20250805-v1:0",
		Vertex:     "claude-opus-4-1@20250805",
		Foundry:    "claude-opus-4-1",
	},
	Opus45: ModelConfig{
		FirstParty: "claude-opus-4-5-20251101",
		Bedrock:    "us.anthropic.claude-opus-4-5-20251101-v1:0",
		Vertex:     "claude-opus-4-5@20251101",
		Foundry:    "claude-opus-4-5",
	},
	Opus46: ModelConfig{
		FirstParty: "claude-opus-4-6",
		Bedrock:    "us.anthropic.claude-opus-4-6-v1",
		Vertex:     "claude-opus-4-6",
		Foundry:    "claude-opus-4-6",
	},
}

func getBuiltinModelStrings(provider APIProvider) ModelStrings {
	switch provider {
	case ProviderBedrock:
		return ModelStrings{
			Haiku35:  allModelConfigs.Haiku35.Bedrock,
			Haiku45:  allModelConfigs.Haiku45.Bedrock,
			Sonnet35: allModelConfigs.Sonnet35.Bedrock,
			Sonnet37: allModelConfigs.Sonnet37.Bedrock,
			Sonnet40: allModelConfigs.Sonnet40.Bedrock,
			Sonnet45: allModelConfigs.Sonnet45.Bedrock,
			Sonnet46: allModelConfigs.Sonnet46.Bedrock,
			Opus40:   allModelConfigs.Opus40.Bedrock,
			Opus41:   allModelConfigs.Opus41.Bedrock,
			Opus45:   allModelConfigs.Opus45.Bedrock,
			Opus46:   allModelConfigs.Opus46.Bedrock,
		}
	case ProviderVertex:
		return ModelStrings{
			Haiku35:  allModelConfigs.Haiku35.Vertex,
			Haiku45:  allModelConfigs.Haiku45.Vertex,
			Sonnet35: allModelConfigs.Sonnet35.Vertex,
			Sonnet37: allModelConfigs.Sonnet37.Vertex,
			Sonnet40: allModelConfigs.Sonnet40.Vertex,
			Sonnet45: allModelConfigs.Sonnet45.Vertex,
			Sonnet46: allModelConfigs.Sonnet46.Vertex,
			Opus40:   allModelConfigs.Opus40.Vertex,
			Opus41:   allModelConfigs.Opus41.Vertex,
			Opus45:   allModelConfigs.Opus45.Vertex,
			Opus46:   allModelConfigs.Opus46.Vertex,
		}
	case ProviderFoundry:
		return ModelStrings{
			Haiku35:  allModelConfigs.Haiku35.Foundry,
			Haiku45:  allModelConfigs.Haiku45.Foundry,
			Sonnet35: allModelConfigs.Sonnet35.Foundry,
			Sonnet37: allModelConfigs.Sonnet37.Foundry,
			Sonnet40: allModelConfigs.Sonnet40.Foundry,
			Sonnet45: allModelConfigs.Sonnet45.Foundry,
			Sonnet46: allModelConfigs.Sonnet46.Foundry,
			Opus40:   allModelConfigs.Opus40.Foundry,
			Opus41:   allModelConfigs.Opus41.Foundry,
			Opus45:   allModelConfigs.Opus45.Foundry,
			Opus46:   allModelConfigs.Opus46.Foundry,
		}
	default:
		return ModelStrings{
			Haiku35:  allModelConfigs.Haiku35.FirstParty,
			Haiku45:  allModelConfigs.Haiku45.FirstParty,
			Sonnet35: allModelConfigs.Sonnet35.FirstParty,
			Sonnet37: allModelConfigs.Sonnet37.FirstParty,
			Sonnet40: allModelConfigs.Sonnet40.FirstParty,
			Sonnet45: allModelConfigs.Sonnet45.FirstParty,
			Sonnet46: allModelConfigs.Sonnet46.FirstParty,
			Opus40:   allModelConfigs.Opus40.FirstParty,
			Opus41:   allModelConfigs.Opus41.FirstParty,
			Opus45:   allModelConfigs.Opus45.FirstParty,
			Opus46:   allModelConfigs.Opus46.FirstParty,
		}
	}
}

func GetModelStrings() ModelStrings {
	return getBuiltinModelStrings(GetAPIProvider())
}

