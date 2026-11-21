ForeverScribeDB = {
	["schema"] = 1,
	["addonVersion"] = "0.10.1",
	["settings"] = {},
	["chars"] = {
		[":Testpriest"] = {
			["key"] = ":Testpriest",
			["snapshot"] = {
				["identity"] = {
					["name"] = "Testpriest",
					["realm"] = "",
					["class"] = "PRIEST",
					["race"] = "Human",
					["faction"] = "Alliance",
					["client"] = {
						["version"] = "9.9.9",
						["build"] = "99999",
						["toc"] = "99999",
					},
				},
				["level"] = 24,
				["xp"] = 1000,
				["xpMax"] = 8000,
				["rested"] = 6,
				["money"] = 20500,
				["capturedAt"] = 1700100000,
				["addonVersion"] = "0.10.1",
				["bags"] = {
					["items"] = {
						[100401] = {
							["itemID"] = 100401,
							["count"] = 3,
							["name"] = "Test Bandage",
						},
					},
				},
				["skills"] = {
					{
						["name"] = "Tailoring",
						["skillID"] = 197,
						["rank"] = 90,
						["max"] = 150,
						["group"] = "Professions",
					}, -- [1]
					{
						["name"] = "Cooking",
						["skillID"] = 185,
						["rank"] = 40,
						["max"] = 75,
						["group"] = "Secondary Skills",
					}, -- [2]
					{
						["name"] = "tailoring",
						["skillID"] = 197,
						["rank"] = 90,
						["max"] = 150,
						["group"] = "Primary Skills",
					}, -- [3]
					{
						["name"] = "Wands",
						["rank"] = 100,
						["max"] = 150,
						["group"] = "Weapon Skills",
					}, -- [4]
				},
				["professions"] = {
					{
						["name"] = "Tailoring",
						["skillID"] = 197,
						["rank"] = 90,
						["max"] = 150,
					}, -- [1]
				},
				["talents"] = {
					["system"] = "traits",
					["configID"] = 1,
					["loadout"] = "Test Loadout",
					["unspent"] = 1,
					["trees"] = {
						{
							["treeID"] = 1,
							["name"] = "Test Priest",
							["points"] = 14,
							["currency"] = {
								["spent"] = 14,
								["unspent"] = 1,
								["max"] = 15,
								["spentInTree"] = 14,
							},
							["groups"] = {
								{
									["groupID"] = 11,
									["name"] = "Test Holy",
									["order"] = 2,
									["spent"] = 0,
								}, -- [1]
								{
									["groupID"] = 10,
									["name"] = "Test Discipline",
									["order"] = 1,
									["spent"] = 14,
								}, -- [2]
								{
									["groupID"] = 12,
									["name"] = "Test Shadow Magic",
									["order"] = 3,
									["spent"] = 0,
								}, -- [3]
							},
							["talents"] = {
								{
									["nodeID"] = 6001,
									["rank"] = 5,
									["max"] = 5,
									["spellID"] = 910001,
									["name"] = "Test Inner Focus",
									["group"] = "Test Discipline",
								}, -- [1]
							},
						}, -- [1]
					},
				},
			},
			["reputation"] = {
				["factions"] = {},
			},
		},
	},
}
