package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/mreiferson/go-options"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	flagSet := flag.NewFlagSet("oauth2_proxy", flag.ExitOnError)

	emailDomains := StringArray{}
	upstreams := StringArray{}
	skipAuthRegex := StringArray{}

	config := flagSet.String("config", "", "path to config file")
	showVersion := flagSet.Bool("version", false, "print version string")

	flagSet.String("http-address", "127.0.0.1:4180", "[http://]<addr>:<port> or unix://<path> to listen on for HTTP clients")
	flagSet.String("https-address", ":443", "<addr>:<port> to listen on for HTTPS clients")
	flagSet.String("tls-cert", "", "path to certificate file")
	flagSet.String("tls-key", "", "path to private key file")
	flagSet.String("redirect-url", "", "the OAuth Redirect URL. ie: \"https://internalapp.yourcompany.com/oauth2/callback\"")
	flagSet.Var(&upstreams, "upstream", "the http url(s) of the upstream endpoint. If multiple, routing is based on path")
	flagSet.Bool("pass-basic-auth", true, "pass HTTP Basic Auth, X-Forwarded-User and X-Forwarded-Email information to upstream")
	flagSet.Bool("pass-access-token", false, "pass OAuth access_token to upstream via X-Forwarded-Access-Token header")
	flagSet.Bool("pass-host-header", true, "pass the request Host Header to upstream")
	flagSet.Var(&skipAuthRegex, "skip-auth-regex", "bypass authentication for requests path's that match (may be given multiple times)")

	flagSet.Var(&emailDomains, "email-domain", "authenticate emails with the specified domain (may be given multiple times). Use * to authenticate any email")
	flagSet.String("github-org", "", "restrict logins to members of this organisation")
	flagSet.String("github-team", "", "restrict logins to members of this team")
	flagSet.String("client-id", "", "the OAuth Client ID: ie: \"123456.apps.googleusercontent.com\"")
	flagSet.String("client-secret", "", "the OAuth Client Secret")
	flagSet.String("authenticated-emails-file", "", "authenticate against emails via file (one per line)")
	flagSet.String("htpasswd-file", "", "additionally authenticate against a htpasswd file. Entries must be created with \"htpasswd -s\" for SHA encryption")
	flagSet.Bool("display-htpasswd-form", true, "display username / password login form if an htpasswd file is provided")
	flagSet.String("custom-templates-dir", "", "path to custom html templates")
	flagSet.String("proxy-prefix", "/oauth2", "the url root path that this proxy should be nested under (e.g. /<oauth2>/sign_in)")

	flagSet.String("cookie-name", "_oauth2_proxy", "the name of the cookie that the oauth_proxy creates")
	flagSet.String("cookie-secret", "", "the seed string for secure cookies")
	flagSet.String("cookie-domain", "", "an optional cookie domain to force cookies to (ie: .yourcompany.com)*")
	flagSet.Duration("cookie-expire", time.Duration(168)*time.Hour, "expire timeframe for cookie")
	flagSet.Duration("cookie-refresh", time.Duration(0), "refresh the cookie when less than this much time remains before expiration; 0 to disable")
	flagSet.Bool("cookie-secure", true, "set secure (HTTPS) cookie flag")
	flagSet.Bool("cookie-httponly", true, "set HttpOnly cookie flag")

	flagSet.Bool("request-logging", true, "Log requests to stdout")

	flagSet.String("provider", "google", "OAuth provider")
	flagSet.String("login-url", "", "Authentication endpoint")
	flagSet.String("redeem-url", "", "Token redemption endpoint")
	flagSet.String("profile-url", "", "Profile access endpoint")
	flagSet.String("validate-url", "", "Access token validation endpoint")
	flagSet.String("scope", "", "Oauth scope specification")

	flagSet.Parse(os.Args[1:])

	if *showVersion {
		fmt.Printf("oauth2_proxy v%s (built with %s)\n", VERSION, runtime.Version())
		return
	}

	opts := NewOptions()

	cfg := make(EnvOptions)
	if *config != "" {
		_, err := toml.DecodeFile(*config, &cfg)
		if err != nil {
			log.Fatalf("ERROR: failed to load config file %s - %s", *config, err)
		}
	}
	cfg.LoadEnvForStruct(opts)
	options.Resolve(opts, flagSet, cfg)

	err := opts.Validate()
	if err != nil {
		log.Printf("%s", err)
		os.Exit(1)
	}

	validator := NewValidator(opts.EmailDomains, opts.AuthenticatedEmailsFile)
	oauthproxy := NewOauthProxy(opts, validator)

	if len(opts.EmailDomains) != 0 && opts.AuthenticatedEmailsFile == "" {
		if len(opts.EmailDomains) > 1 {
			oauthproxy.SignInMessage = fmt.Sprintf("Authenticate using one of the following domains: %v", strings.Join(opts.EmailDomains, ", "))
		} else if opts.EmailDomains[0] != "*" {
			oauthproxy.SignInMessage = fmt.Sprintf("Authenticate using %v", opts.EmailDomains[0])
		}
	}

	if opts.HtpasswdFile != "" {
		log.Printf("using htpasswd file %s", opts.HtpasswdFile)
		oauthproxy.HtpasswdFile, err = NewHtpasswdFromFile(opts.HtpasswdFile)
		oauthproxy.DisplayHtpasswdForm = opts.DisplayHtpasswdForm
		if err != nil {
			log.Fatalf("FATAL: unable to open %s %s", opts.HtpasswdFile, err)
		}
	}

	s := &Server{
		Handler: LoggingHandler(os.Stdout, oauthproxy, opts.RequestLogging),
		Opts:    opts,
	}
	s.ListenAndServe()
}
