package main

import "testing"

func TestSkillLinkOnlyQueuesExactVersion(t *testing.T) {
	link := "luckytv://skills/install?id=official-storyboard-text&version=r_123"
	route, err := skillLinkRoute(link)
	if err != nil || route != "/skills?releaseId=r_123&skillId=official-storyboard-text" {
		t.Fatalf("%q %v", route, err)
	}
	app := newDesktopApp(t.TempDir())
	app.queueSkillLink(link)
	if got := app.ConsumeSkillLink(); got != route {
		t.Fatalf("lost startup link: %s", got)
	}
	if app.ConsumeSkillLink() != "" {
		t.Fatal("link consumed twice")
	}
	if app.runtime() != nil {
		t.Fatal("link started backend or installed a package")
	}
	if err := validateDesktopArgs([]string{link}); err != nil {
		t.Fatal(err)
	}
}

func TestSkillLinkRejectsAmbiguousOrExecutableInput(t *testing.T) {
	for _, raw := range []string{"https://skills/install?id=a&version=b", "luckytv://evil/install?id=a&version=b", "luckytv://skills/install?id=a", "luckytv://skills/install?id=a&version=b&version=c", "luckytv://skills/install?id=a&version=b&install=true", "luckytv://skills/install?id=..%2fa&version=b", "luckytv://skills/install?id=a&version=b#auto", "luckytv://user@skills/install?id=a&version=b", "luckytv://skills/install?id=a&version=", "luckytv://skills/install?id=a&version=b%00"} {
		if _, err := skillLinkRoute(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	if skillInstanceID("one") == skillInstanceID("two") {
		t.Fatal("isolated data directories share lock")
	}
}

func TestSkillWebsiteLinksStayOnTheSkillSite(t *testing.T) {
	for _, raw := range []string{"https://luckytv.app/skills/mine", "https://luckytv.app/skills/authorize?user_code=ABCDE", "http://127.0.0.1:4186/skills/submissions/d_1"} {
		if !validSkillWebsite(raw) {
			t.Errorf("rejected %q", raw)
		}
	}
	for _, raw := range []string{"https://evil.test/skills", "https://luckytv.app.evil.test/skills", "https://luckytv.app/account", "javascript:alert(1)", "https://user@luckytv.app/skills", "file:///skills", "https://luckytv.app/skills/../account", "https://luckytv.app/skills/%2e%2e/account", "https://luckytv.app/skills/%5c..%5caccount"} {
		if validSkillWebsite(raw) {
			t.Errorf("accepted %q", raw)
		}
	}
}
