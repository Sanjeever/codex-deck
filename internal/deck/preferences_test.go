package deck

import "testing"

func TestPreferencesPersistWithoutChangingCredentials(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	if err := s.SavePreferences(Preferences{Privacy: true, Sort: "quota"}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := OpenStore(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	d := reloaded.Snapshot()
	if !d.Preferences.Privacy || d.Preferences.Sort != "quota" || d.Accounts[0].Tokens.AccessToken != a.Tokens.AccessToken {
		t.Fatal("偏好设置未保存或改写凭据")
	}
	if err := s.SavePreferences(Preferences{Sort: "invalid"}); err == nil {
		t.Fatal("接受未知排序")
	}
}

func TestQuotaWindowSortPreferencesPersist(t *testing.T) {
	s := testService(t)
	for _, sort := range []string{"five_hour", "weekly"} {
		if err := s.SavePreferences(Preferences{Sort: sort}); err != nil {
			t.Fatal(err)
		}
		reloaded, err := OpenStore(s.store.path)
		if err != nil {
			t.Fatal(err)
		}
		if reloaded.Snapshot().Preferences.Sort != sort {
			t.Fatalf("排序未保存: %s", sort)
		}
	}
}
