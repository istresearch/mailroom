module github.com/nyaruka/mailroom

require (
	github.com/Masterminds/semver v1.5.0
	github.com/apex/log v1.1.4
	github.com/aws/aws-sdk-go v1.33.5
	github.com/buger/jsonparser v0.0.0-20200322175846-f7e751efca13
	github.com/certifi/gocertifi v0.0.0-20200211180108-c7c1fbc02894 // indirect
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/edganiukov/fcm v0.4.0
	github.com/getsentry/raven-go v0.1.2-0.20190125112653-238ebd86338d // indirect
	github.com/go-chi/chi v3.3.3+incompatible
	github.com/golang/protobuf v1.4.0
	github.com/gomodule/redigo v2.0.0+incompatible
	github.com/gorilla/schema v1.1.0
	github.com/jmoiron/sqlx v1.2.0
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lib/pq v1.4.0
	github.com/mattn/go-sqlite3 v1.10.0 // indirect
	github.com/nyaruka/ezconf v0.2.1
	github.com/nyaruka/gocommon v1.2.0
	github.com/nyaruka/goflow v0.94.2
	github.com/nyaruka/librato v1.0.0
	github.com/nyaruka/logrus_sentry v0.8.2-0.20190129182604-c2962b80ba7d
	github.com/nyaruka/null v1.2.0
	github.com/olivere/elastic/v7 v7.0.19
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.9.1
	github.com/shopspring/decimal v0.0.0-20180709203117-cd690d0c9e24
	github.com/sirupsen/logrus v1.5.0
	github.com/stretchr/testify v1.5.1
	gopkg.in/go-playground/validator.v9 v9.31.0
	gopkg.in/mail.v2 v2.3.1
)

go 1.14

replace github.com/nyaruka/goflow v0.94.2 => github.com/istresearch/goflow v0.94.2-devtest
