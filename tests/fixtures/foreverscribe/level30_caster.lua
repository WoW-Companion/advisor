ForeverScribeDB = {
	["schema"] = 1,
	["addonVersion"] = "0.10.1",
	["settings"] = {
		["played"] = true,
	},
	["chars"] = {
		["Testrealm:Testmage"] = {
			["key"] = "Testrealm:Testmage",
			["events"] = {},
			["sessions"] = {},
			["snapshot"] = {
				["identity"] = {
					["name"] = "Testmage",
					["realm"] = "Testrealm",
					["class"] = "MAGE",
					["classID"] = 8,
					["race"] = "Gnome",
					["raceID"] = 7,
					["faction"] = "Alliance",
					["guild"] = "Test Guild",
				},
				["level"] = 30,
				["xp"] = 12345,
				["xpMax"] = 56000,
				["rested"] = 2800,
				["money"] = 1234567,
				["capturedAt"] = 1700100000,
				["addonVersion"] = "0.10.1",
				["equipment"] = {
					["head"] = {
						["slot"] = 1,
						["itemID"] = 100101,
						["link"] = "|cff1eff00|Hitem:100101:2999:::::::30:::::|h[Test Hood of Wisdom]|h|r",
						["name"] = "Test Hood of Wisdom",
						["ilvl"] = 28,
						["quality"] = 2,
						["reqLevel"] = 25,
						["enchant"] = 2999,
						["dur"] = 40,
						["durMax"] = 45,
					},
					["chest"] = {
						["slot"] = 5,
						["itemID"] = 100102,
						["link"] = "|cff0070dd|Hitem:100102::::::::30:::::|h[Test Robe of \"Quotes\"]|h|r",
						["name"] = "Test Robe of \"Quotes\"",
						["ilvl"] = 30,
						["quality"] = 3,
					},
					["mainhand"] = {
						["slot"] = 16,
						["itemID"] = 100103,
						["link"] = "|cff0070dd|Hitem:100103:3834:::::::30:::::|h[Test Staff]|h|r",
						["name"] = "Test Staff",
						["ilvl"] = 31,
						["quality"] = 3,
						["enchant"] = 3834,
						["dur"] = 90,
						["durMax"] = 90,
					},
				},
				["bags"] = {
					["items"] = {
						[100201] = {
							["itemID"] = 100201,
							["count"] = 20,
							["name"] = "Test Linen Cloth",
							["quality"] = 1,
						},
						[100202] = {
							["itemID"] = 100202,
							["count"] = 5,
							["name"] = "Test Water",
						},
					},
					["free"] = 10,
					["slots"] = 24,
				},
				["quests"] = {
					{
						["questID"] = 90001,
						["title"] = "Test Quest: Fetch",
						["level"] = 29,
						["zone"] = "Testzone",
						["complete"] = false,
						["obj"] = {
							{
								["text"] = "Test Boars slain: 3/8",
								["have"] = 3,
								["need"] = 8,
								["done"] = false,
								["type"] = "monster",
							}, -- [1]
						},
					}, -- [1]
					{
						["questID"] = 90002,
						["title"] = "Test Quest: Report",
						["level"] = 30,
						["zone"] = "Testzone",
						["complete"] = true,
						["obj"] = {},
					}, -- [2]
				},
				["questsCompleted"] = { 90010, 90011, 90012 },
				["skills"] = {
					{
						["name"] = "Tailoring",
						["skillID"] = 197,
						["rank"] = 150,
						["max"] = 225,
						["group"] = "Professions",
					}, -- [1]
					{
						["name"] = "Enchanting",
						["skillID"] = 333,
						["rank"] = 75,
						["max"] = 150,
						["group"] = "Professions",
					}, -- [2]
					{
						["name"] = "Staves",
						["rank"] = 120,
						["max"] = 150,
						["group"] = "Weapon Skills",
					}, -- [3]
				},
				["talents"] = {
					["system"] = "traits",
					["configID"] = 1,
					["loadout"] = "Test Loadout",
					["unspent"] = 2,
					["trees"] = {
						{
							["treeID"] = 1,
							["name"] = "Test Class Tree",
							["points"] = 3,
							["talents"] = {
								{
									["nodeID"] = 5001,
									["rank"] = 2,
									["max"] = 2,
									["spellID"] = 900001,
									["name"] = "Test Arcane Focus",
								}, -- [1]
								{
									["nodeID"] = 5002,
									["rank"] = 1,
									["max"] = 3,
									["name"] = "Test Fire Focus",
								}, -- [2]
								{
									["nodeID"] = 5003,
									["rank"] = 0,
									["max"] = 1,
									["name"] = "Test Untaken",
								}, -- [3]
							},
						}, -- [1]
					},
				},
			},
			["reputation"] = {
				["factions"] = {
					{
						["id"] = 72,
						["name"] = "Test Stormwind",
						["standing"] = 5,
						["value"] = 4200,
						["min"] = 3000,
						["max"] = 9000,
						["atWar"] = false,
						["group"] = "Alliance",
					}, -- [1]
				},
				["count"] = 1,
				["scannedAt"] = 1700100000,
			},
			["bank"] = {
				["items"] = {
					[100301] = {
						["itemID"] = 100301,
						["count"] = 200,
						["name"] = "Test Copper Ore",
					},
				},
				["totals"] = {
					[100301] = 200,
				},
				["scannedAt"] = 1700090000,
			},
		},
	},
}
