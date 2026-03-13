package reader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tallybridge/connector/tally"
)

// tallyXMLServer wraps s in a minimal HTTP server that returns the given XML.
func tallyXMLServer(t *testing.T, responseXML string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responseXML))
	}))
}

// ---- parseCompanyResponse ---------------------------------------------------
// Note: The NATIVEMETHOD format returns NAME as an XML *attribute* on <COMPANY>,
// not as a child element. The parser uses xml:"NAME,attr".

func TestParseCompanyResponse_MultipleCompanies(t *testing.T) {
	raw := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<ENVELOPE>
  <BODY>
    <DATA>
      <COLLECTION>
        <COMPANY NAME="Demo Trading Co"/>
        <COMPANY NAME="Sample Store"/>
      </COLLECTION>
    </DATA>
  </BODY>
</ENVELOPE>`)

	companies, err := parseCompanyResponse(raw)
	if err != nil {
		t.Fatalf("parseCompanyResponse: unexpected error: %v", err)
	}
	if len(companies) != 2 {
		t.Fatalf("expected 2 companies, got %d", len(companies))
	}

	if companies[0].Name != "Demo Trading Co" {
		t.Errorf("Name = %q, want %q", companies[0].Name, "Demo Trading Co")
	}
	if companies[1].Name != "Sample Store" {
		t.Errorf("Name = %q, want %q", companies[1].Name, "Sample Store")
	}
}

func TestParseCompanyResponse_EmptyCollection(t *testing.T) {
	raw := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<ENVELOPE>
  <BODY>
    <DATA>
      <COLLECTION/>
    </DATA>
  </BODY>
</ENVELOPE>`)

	companies, err := parseCompanyResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(companies) != 0 {
		t.Errorf("expected 0 companies, got %d", len(companies))
	}
}

func TestParseCompanyResponse_InvalidXML(t *testing.T) {
	raw := []byte(`not xml at all`)
	_, err := parseCompanyResponse(raw)
	if err == nil {
		t.Error("expected error for invalid XML, got nil")
	}
}

// ---- ListCompanies (integration with mock HTTP server) ----------------------

func TestListCompanies_MockServer(t *testing.T) {
	srv := tallyXMLServer(t, `<?xml version="1.0" encoding="UTF-8"?>
<ENVELOPE>
  <BODY>
    <DATA>
      <COLLECTION>
        <COMPANY NAME="ACME Pvt Ltd"/>
      </COLLECTION>
    </DATA>
  </BODY>
</ENVELOPE>`)
	defer srv.Close()

	client := tally.NewClient(tally.WithAddr(srv.URL))
	companies, err := ListCompanies(context.Background(), client)
	if err != nil {
		t.Fatalf("ListCompanies: %v", err)
	}
	if len(companies) != 1 {
		t.Fatalf("expected 1 company, got %d", len(companies))
	}
	if companies[0].Name != "ACME Pvt Ltd" {
		t.Errorf("Name = %q, want %q", companies[0].Name, "ACME Pvt Ltd")
	}
}

// ---- Heartbeat --------------------------------------------------------------

func TestHeartbeat_Online(t *testing.T) {
	srv := tallyXMLServer(t, `<?xml version="1.0" encoding="UTF-8"?>
<ENVELOPE>
  <BODY>
    <DATA>
      <COLLECTION>
        <COMPANY NAME="ZenPivot Trading"/>
      </COLLECTION>
    </DATA>
  </BODY>
</ENVELOPE>`)
	defer srv.Close()

	client := tally.NewClient(tally.WithAddr(srv.URL))
	info := Heartbeat(context.Background(), client)

	if !info.Online {
		t.Error("expected Online=true")
	}
	if info.OpenCompany != "ZenPivot Trading" {
		t.Errorf("OpenCompany = %q, want %q", info.OpenCompany, "ZenPivot Trading")
	}
	if info.CheckedAt.IsZero() {
		t.Error("CheckedAt should not be zero")
	}
}

func TestHeartbeat_Offline(t *testing.T) {
	// Point to a port with nothing listening.
	client := tally.NewClient(
		tally.WithAddr("http://localhost:19999"),
		tally.WithTimeout(500*time.Millisecond),
	)
	info := Heartbeat(context.Background(), client)

	if info.Online {
		t.Error("expected Online=false for unreachable port")
	}
}

func TestHeartbeat_EmptyCompany(t *testing.T) {
	srv := tallyXMLServer(t, `<?xml version="1.0" encoding="UTF-8"?>
<ENVELOPE><BODY><DATA><COLLECTION/></DATA></BODY></ENVELOPE>`)
	defer srv.Close()

	client := tally.NewClient(tally.WithAddr(srv.URL))
	info := Heartbeat(context.Background(), client)

	if !info.Online {
		t.Error("expected Online=true — Tally responded, just no open company")
	}
	if info.OpenCompany != "" {
		t.Errorf("OpenCompany = %q, want empty", info.OpenCompany)
	}
}
