// Copyright 2015 Canonical Ltd.

package keystone_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/juju/idmclient/params"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/yaml.v2"

	"github.com/CanonicalLtd/blues-identity/config"
	"github.com/CanonicalLtd/blues-identity/idp"
	"github.com/CanonicalLtd/blues-identity/idp/idptest"
	keystoneidp "github.com/CanonicalLtd/blues-identity/idp/keystone"
	"github.com/CanonicalLtd/blues-identity/idp/keystone/internal/mockkeystone"
)

type tokenV3Suite struct {
	server *mockkeystone.Server
	params keystoneidp.Params
	idp    idp.IdentityProvider
}

var _ = gc.Suite(&tokenV3Suite{})

func (s *tokenV3Suite) SetUpTest(c *gc.C) {
	s.server = mockkeystone.NewServer()
	s.params = keystoneidp.Params{
		Name:        "openstack",
		Description: "OpenStack",
		Domain:      "openstack",
		URL:         s.server.URL,
	}
	s.server.AuthTokensFunc = testAuthTokens
	s.server.UserGroupsFunc = testUserGroups
	s.idp = keystoneidp.NewV3TokenIdentityProvider(s.params)
}

func (s *tokenV3Suite) TearDownTest(c *gc.C) {
	s.server.Close()
}

func (s *tokenV3Suite) TestKeystoneV3TokenIdentityProviderInteractive(c *gc.C) {
	c.Assert(s.idp.Interactive(), gc.Equals, false)
}

func (s *tokenV3Suite) TestKeystoneV3TokenIdentityProviderHandle(c *gc.C) {
	var tok keystoneidp.Token
	tok.Login.ID = "789"
	body, err := json.Marshal(tok)
	c.Assert(err, gc.IsNil)
	req, err := http.NewRequest("POST", "https://idp.test/login?waitid=1", bytes.NewReader(body))
	c.Assert(err, gc.IsNil)
	req.Header.Set("Content-Type", "application/json")
	tc := &idptest.TestContext{
		Context:   context.Background(),
		URLPrefix: "https://idp.test",
		Bakery_:   bakery.New(bakery.BakeryParams{}),
		Request:   req,
	}
	rr := httptest.NewRecorder()
	s.idp.Handle(tc, rr, tc.Request)
	idptest.AssertLoginSuccess(c, tc, "testuser@openstack")
	idptest.AssertUser(c, tc, &params.User{
		Username:   params.Username("testuser@openstack"),
		ExternalID: "123@openstack",
		IDPGroups:  []string{"abc_group@openstack"},
	})
	c.Assert(rr.Body.String(), gc.Equals, "login successful as user testuser@openstack\n")
}

func (s *tokenV3Suite) TestKeystoneV3TokenIdentityProviderHandleBadToken(c *gc.C) {
	var tok keystoneidp.Token
	tok.Login.ID = "012"
	body, err := json.Marshal(tok)
	c.Assert(err, gc.IsNil)
	req, err := http.NewRequest("POST", "https://idp.test/login?waitid=1", bytes.NewReader(body))
	c.Assert(err, gc.IsNil)
	req.Header.Set("Content-Type", "application/json")
	tc := &idptest.TestContext{
		Context:   context.Background(),
		URLPrefix: "https://idp.test",
		Bakery_:   bakery.New(bakery.BakeryParams{}),
		Request:   req,
	}
	rr := httptest.NewRecorder()
	s.idp.Handle(tc, rr, tc.Request)
	idptest.AssertLoginFailure(c, tc, `cannot log in: Post http.*: The request you have made requires authentication.`)
}

func (s *tokenV3Suite) TestKeystoneV3TokenIdentityProviderHandleBadRequest(c *gc.C) {
	req, err := http.NewRequest("POST", "https://idp.test/login?waitid=1", strings.NewReader("{"))
	c.Assert(err, gc.IsNil)
	req.Header.Set("Content-Type", "application/json")
	tc := &idptest.TestContext{
		Context:   context.Background(),
		URLPrefix: "https://idp.test",
		Bakery_:   bakery.New(bakery.BakeryParams{}),
		Request:   req,
	}
	rr := httptest.NewRecorder()
	s.idp.Handle(tc, rr, tc.Request)
	idptest.AssertLoginFailure(c, tc, `cannot unmarshal login request: cannot unmarshal into field: cannot unmarshal request body: unexpected end of JSON input`)
}

func (s *tokenV3Suite) TestRegisterConfig(c *gc.C) {
	input := `
identity-providers:
 - type: keystonev3_token
   name: openstackv3_3
   url: https://example.com/keystone
`
	var conf config.Config
	err := yaml.Unmarshal([]byte(input), &conf)
	c.Assert(err, gc.IsNil)
	c.Assert(conf.IdentityProviders, gc.HasLen, 1)
	c.Assert(conf.IdentityProviders[0].Name(), gc.Equals, "openstackv3_3")
}