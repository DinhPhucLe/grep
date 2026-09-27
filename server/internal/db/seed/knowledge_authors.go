package seed

import "cortisol-server/internal/orgknowledge"

// DemoKnowledgeAuthors returns NovaPay and AtlasHealth employees for knowledge attribution.
func DemoKnowledgeAuthors() []orgknowledge.DemoAuthor {
	cast := DemoCastIDs()
	nova := orgSpec{
		orgByte: orgNovaPay,
		names: []string{
			"Alex Rivera", "Jordan Kim", "Casey Nguyen", "Riley Patel", "Morgan Ellis",
			"Avery Brooks", "Quinn Morales", "Reese Cooper", "Harper Diaz", "Rowan Blake",
			"Cameron Soto", "Drew Vance", "Jamie Ortiz", "Parker Chen", "Taylor Brooks",
			"Skyler Reed", "Finley Hayes", "Emerson Cruz", "Dakota Price", "Phoenix Lane",
			"Sage Romero", "Blair Kent", "Elliot Nash", "Remy Cole", "Logan Pierce",
		},
	}
	atlas := orgSpec{
		orgByte: orgAtlasHealth,
		names: []string{
			"Sam Okonkwo", "Nina Vargas", "Chris Adeyemi", "Priya Shah", "Marcus Bell",
			"Elena Rossi", "Omar Farouk", "Lila Cho", "Ben Travers", "Sofia Mendes",
			"Kai Nakamura", "Amelia Frost", "Noah Berg", "Isla Quinn", "Leo Hartmann",
			"Maya Okada", "Ethan Brooks", "Zoe Keller", "Luke Anders", "Aria Singh",
			"Owen Clarke", "Nora Jimenez", "Felix Braun", "Chloe Park", "Hugo Martins",
		},
	}
	out := make([]orgknowledge.DemoAuthor, 0, employeesPerOrg*2)
	for local := byte(1); local <= employeesPerOrg; local++ {
		out = append(out, orgknowledge.DemoAuthor{
			UserID: seedID(kindUser, nova.orgByte, local, 0, 0).Hex(),
			Name:   nova.names[local-1],
			OrgID:  cast.NovaPayOrgID.Hex(),
		})
	}
	for local := byte(1); local <= employeesPerOrg; local++ {
		out = append(out, orgknowledge.DemoAuthor{
			UserID: seedID(kindUser, atlas.orgByte, local, 0, 0).Hex(),
			Name:   atlas.names[local-1],
			OrgID:  cast.AtlasHealthOrgID.Hex(),
		})
	}
	return out
}
