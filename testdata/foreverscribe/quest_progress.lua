ForeverScribeDB = {
	["schema"] = 1,
	["addonVersion"] = "0.10.1",
	["chars"] = {
		["Testrealm:Testscout"] = {
			["snapshot"] = {
				["identity"] = { ["name"] = "Testscout", ["realm"] = "Testrealm", ["class"] = "MAGE", ["race"] = "Human" },
				["level"] = 9,
				["capturedAt"] = 1700100000,
				["quests"] = {
					{
						["questID"] = 91001, ["title"] = "Test Claws", ["level"] = 9, ["zone"] = "Testzone",
						["obj"] = {
							{ ["text"] = "Test Claws: 3/8", ["have"] = 3, ["need"] = 8, ["done"] = false, ["type"] = "item" },
						},
					},
					{
						["questID"] = 91002, ["title"] = "Test Boars", ["level"] = 9, ["zone"] = "Otherzone",
						["obj"] = {
							{ ["text"] = "Boars slain: 7/8", ["have"] = 7, ["need"] = 8, ["done"] = false, ["type"] = "monster" },
							{ ["text"] = "Test Tusk", ["have"] = 1, ["need"] = 1, ["done"] = true, ["type"] = "item" },
							{ ["text"] = "Test Hide", ["have"] = 1, ["need"] = 1, ["done"] = true, ["type"] = "item" },
							{ ["text"] = "Test Fang", ["have"] = 1, ["need"] = 1, ["done"] = true, ["type"] = "item" },
						},
					},
					{
						["questID"] = 91003, ["title"] = "Test Bare", ["level"] = 9, ["zone"] = "Testzone",
						["obj"] = { { ["text"] = "Speak to the Test Elder" } },
					},
					{
						["questID"] = 91004, ["title"] = "Test Far", ["level"] = 9, ["zone"] = "Otherzone",
					},
				},
			},
			["npcs"] = {
				["9001:100"] = { ["id"] = 9001, ["name"] = "Test Far Giver", ["map"] = 100, ["zone"] = "Testzone", ["subzone"] = "Test Hill", ["x"] = 80.0, ["y"] = 80.0, ["kind"] = "quest", ["t"] = 1699000000, ["n"] = 2 },
				["9002:100"] = { ["id"] = 9002, ["name"] = "Test Near Giver", ["map"] = 100, ["zone"] = "Testzone", ["subzone"] = "Test Dock", ["x"] = 61.6, ["y"] = 20.7, ["kind"] = "quest", ["t"] = 1700090000, ["n"] = 5,
					["spots"] = { { ["x"] = 61.6, ["y"] = 20.7, ["t"] = 1700090000, ["n"] = 5 } } },
				["9003:100"] = { ["id"] = 9003, ["name"] = "Test Gossiper", ["map"] = 100, ["zone"] = "Testzone", ["x"] = 60.0, ["y"] = 20.0, ["kind"] = "gossip", ["t"] = 1700095000 },
				["9004:200"] = { ["id"] = 9004, ["name"] = "Test Other Map", ["map"] = 200, ["zone"] = "Otherzone", ["x"] = 61.0, ["y"] = 21.0, ["kind"] = "quest", ["t"] = 1700000000 },
				["9005:100"] = { ["id"] = 9005, ["name"] = "Test Sparse", ["map"] = 100, ["zone"] = "Testzone", ["kind"] = "quest" },
			},
		},
	},
}
