package main

type UserFlags struct {
	IdentityOverride bool
	FakeDisconnected bool
}

type CharacterFlags struct {
	Dead      bool
	Trunk     bool
	Shell     bool
	Invisible bool
}

func getUserFlags(player map[string]interface{}) UserFlags {
	flags, ok := player["flags"].(int64)

	if ok {
		FakeDisconnected := flags/2 >= 1
		if FakeDisconnected {
			flags -= 2
		}

		IdentityOverride := flags != 0

		return UserFlags{
			IdentityOverride: IdentityOverride,
			FakeDisconnected: FakeDisconnected,
		}
	}

	return UserFlags{}
}

func getCharacterFlags(character map[string]interface{}) CharacterFlags {
	flags, ok := character["flags"].(int64)

	if ok {
		Invisible := flags/8 >= 1
		if Invisible {
			flags -= 8
		}

		Shell := flags/4 >= 1
		if Shell {
			flags -= 4
		}

		Trunk := flags/2 >= 1
		if Trunk {
			flags -= 2
		}

		Dead := flags != 0

		return CharacterFlags{
			Invisible: Invisible,
			Shell:     Shell,
			Trunk:     Trunk,
			Dead:      Dead,
		}
	}

	return CharacterFlags{}
}
