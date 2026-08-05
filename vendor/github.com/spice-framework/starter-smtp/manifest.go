package smtp

import spicestarter "github.com/spice-framework/spice/annotation/sdk/starter"

// Manifest returns SMTP starter compatibility and review metadata.
func Manifest() spicestarter.Manifest {
	return spicestarter.Must(spicestarter.Spec{
		Schema:    spicestarter.Schema,
		ID:        "github.com/spice-framework/starter-smtp",
		Version:   "0.1.0-dev",
		Module:    "github.com/spice-framework/starter-smtp",
		SpiceAPI:  spicestarter.APIVersion,
		MinimumGo: "1.26",
		License:   "Apache-2.0",
		Review:    "docs/dependency-review.md",
		Activation: spicestarter.Activation{
			Mode: spicestarter.ActivationExplicitConstructor,
			EntryPoints: []spicestarter.EntryPoint{
				{
					Package: "github.com/spice-framework/starter-smtp",
					Symbol:  "New",
				},
			},
		},
		Capabilities: []string{"mail.smtp"},
	})
}
