// Copyright 2015 Canonical Ltd.

package keystone_test

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"

	"github.com/juju/httprequest"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	envschemaform "gopkg.in/juju/environschema.v1/form"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery/form"

	"github.com/CanonicalLtd/blues-identity/idp"
	"github.com/CanonicalLtd/blues-identity/idp/idputil"
	"github.com/CanonicalLtd/blues-identity/idp/keystone"
	"github.com/CanonicalLtd/blues-identity/idp/keystone/internal/mockkeystone"
	"github.com/CanonicalLtd/blues-identity/internal/idmtest"
)

type dischargeSuite struct {
	idmtest.DischargeSuite
	server *mockkeystone.Server
	params keystone.Params
}

var _ = gc.Suite(&dischargeSuite{})

func (s *dischargeSuite) SetUpSuite(c *gc.C) {
	s.DischargeSuite.SetUpSuite(c)
	s.server = mockkeystone.NewServer()
	s.params = keystone.Params{
		Name:        "openstack",
		Description: "OpenStack",
		Domain:      "openstack",
		URL:         s.server.URL,
	}
	s.server.TokensFunc = testTokens
	s.server.TenantsFunc = testTenants
}

func (s *dischargeSuite) TearDownSuite(c *gc.C) {
	s.server.Close()
	s.DischargeSuite.TearDownSuite(c)
}

func (s *dischargeSuite) SetUpTest(c *gc.C) {
	s.Params.IdentityProviders = []idp.IdentityProvider{
		keystone.NewIdentityProvider(s.params),
		keystone.NewUserpassIdentityProvider(
			keystone.Params{
				Name:        "form",
				Domain:      s.params.Domain,
				Description: s.params.Description,
				URL:         s.params.URL,
			},
		),
		keystone.NewTokenIdentityProvider(
			keystone.Params{
				Name:        "token",
				Domain:      s.params.Domain,
				Description: s.params.Description,
				URL:         s.params.URL,
			},
		),
	}
	s.DischargeSuite.SetUpTest(c)
}

func (s *dischargeSuite) TestInteractiveDischarge(c *gc.C) {
	s.AssertDischarge(c, idmtest.VisitorFunc(s.visitInteractive))
}

var urlRegexp = regexp.MustCompile(`[Aa][Cc][Tt][Ii][Oo][Nn]="(.*)"`)

func (s *dischargeSuite) visitInteractive(u *url.URL) error {
	resp, err := http.Get(u.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	sm := urlRegexp.FindSubmatch(body)
	if len(sm) < 2 {
		return errgo.Newf("could not find URL: %q", body)
	}
	resp, err = http.PostForm(string(sm[1]), url.Values{
		"username": []string{"testuser"},
		"password": []string{"testpass"},
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errgo.Newf("bad status %q", resp.Status)
	}
	return nil
}

func (s *dischargeSuite) TestFormDischarge(c *gc.C) {
	s.AssertDischarge(c, httpbakery.NewMultiVisitor(
		form.Visitor{
			Filler: keystoneFormFiller{
				username: "testuser",
				password: "testpass",
			},
		},
	))
}

type keystoneFormFiller struct {
	username, password string
}

func (h keystoneFormFiller) Fill(f envschemaform.Form) (map[string]interface{}, error) {
	if _, ok := f.Fields["username"]; !ok {
		return nil, errgo.New("schema has no username")
	}
	if _, ok := f.Fields["password"]; !ok {
		return nil, errgo.New("schema has no password")
	}
	return map[string]interface{}{
		"username": h.username,
		"password": h.password,
	}, nil
}

func (s *dischargeSuite) TestTokenDischarge(c *gc.C) {
	s.AssertDischarge(c, idmtest.VisitorFunc(s.visitToken))
}

type tokenLoginRequest struct {
	httprequest.Route `httprequest:"POST"`
	Token             keystone.Token `httprequest:",body"`
}

func (s *dischargeSuite) visitToken(u *url.URL) error {
	client := &httprequest.Client{}
	var lm map[string]string
	if err := idputil.GetLoginMethods(s.Ctx, client, u, &lm); err != nil {
		return errgo.Mask(err)
	}
	if lm["token"] == "" {
		return errgo.Newf("token login not supported")
	}
	var req tokenLoginRequest
	req.Token.Login.ID = "789"
	if err := client.CallURL(s.Ctx, lm["token"], &req, nil); err != nil {
		return errgo.Mask(err)
	}
	return nil
}
