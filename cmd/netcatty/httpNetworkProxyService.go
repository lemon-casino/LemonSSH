package main

import "github.com/binaricat/netcatty/internal/platform/netpolicy"

type HTTPNetworkProxySettings struct {
	Mode   string `json:"mode"`
	URL    string `json:"url"`
	Bypass string `json:"bypass"`
}

type HTTPNetworkProxyResult struct {
	Success  bool                     `json:"success,omitempty"`
	Settings HTTPNetworkProxySettings `json:"settings"`
	Error    string                   `json:"error,omitempty"`
}

type HTTPNetworkProxyService struct{ policy *netpolicy.Policy }

func newHTTPNetworkProxyService(policy *netpolicy.Policy) *HTTPNetworkProxyService {
	_ = policy.SetHTTPProxy("system", "", "<local>")
	return &HTTPNetworkProxyService{policy: policy}
}

func (s *HTTPNetworkProxyService) Set(settings HTTPNetworkProxySettings) HTTPNetworkProxyResult {
	if err := s.policy.SetHTTPProxy(settings.Mode, settings.URL, settings.Bypass); err != nil {
		return HTTPNetworkProxyResult{Settings: s.Get().Settings, Error: err.Error()}
	}
	result := s.Get()
	result.Success = true
	return result
}

func (s *HTTPNetworkProxyService) Get() HTTPNetworkProxyResult {
	mode, rawURL, bypass := s.policy.HTTPProxy()
	if mode == "" {
		mode = "system"
	}
	if bypass == "" {
		bypass = "<local>"
	}
	return HTTPNetworkProxyResult{Settings: HTTPNetworkProxySettings{Mode: mode, URL: rawURL, Bypass: bypass}}
}
