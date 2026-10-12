package persist

import "testing"

func TestNormalizeIPAddressPreventsPortAndMappedIPv6BanBypass(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"192.0.2.42", "192.0.2.42"}, {"192.0.2.42:54321", "192.0.2.42"},
		{"::ffff:192.0.2.42", "192.0.2.42"}, {"[::ffff:192.0.2.42]:2000", "192.0.2.42"},
		{"2001:db8:0:0::1", "2001:db8::1"}, {"[2001:db8::1]:2000", "2001:db8::1"},
	} {
		got, err := NormalizeIPAddress(tc.input)
		if err != nil || got != tc.want {
			t.Errorf("NormalizeIPAddress(%q) = %q,%v; want %q", tc.input, got, err, tc.want)
		}
	}
	for _, value := range []string{"", "localhost", "192.0.2.999", "192.0.2.1/24", "1; DROP TABLE accounts"} {
		if _, err := NormalizeIPAddress(value); err == nil {
			t.Errorf("非法 IP 未拒絕: %s", value)
		}
	}
}
