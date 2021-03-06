This project is the source for http://godoc.org/

[![GoDoc](https://godoc.org/github.com/golang/gddo?status.svg)](http://godoc.org/github.com/golang/gddo)

The code in this project is designed to be used by godoc.org. Send mail to
golang-dev@googlegroups.com if you want to discuss other uses of the code.

Feedback
--------

Send ideas and questions to golang-dev@googlegroups.com. Request features and report bugs
using the [GitHub Issue Tracker](https://github.com/golang/gddo/issues/new). 


Contributions
-------------
Contributions to this project are welcome, though please send mail before
starting work on anything major. Contributors retain their copyright, so we
need you to fill out a short form before we can accept your contribution:
https://developers.google.com/open-source/cla/individual

Development Environment Setup
-----------------------------

- Install and run [Redis 2.8.x](http://redis.io/download). The redis.conf file included in the Redis distribution is suitable for development.
- Install Go 1.4.
- Install and run the server:

        $ go get github.com/golang/gddo/gddo-server
        $ gddo-server

- Browse to [http://localhost:8080/](http://localhost:8080/)
- Enter an import path to have the server retrieve & display a package's documentation

Optional:

- Create the file gddo-server/config.go using the template in [gddo-server/config.go.template](gddo-server/config.go.template).

API
---

The GoDoc API is comprised of these endpoints:

**api.godoc.org/search?q=`Query`**&mdash;Returns search results for Query, in JSON format.

```json
{
	"results": [
		{
			"path": "import/path/one",
			"synopsis": "Package synopsis is here, if present."
		},
		{
			"path": "import/path/two",
			"synopsis": "Package synopsis is here, if present."
		}
	]
}
```

**api.godoc.org/packages**&mdash;Returns all indexed packages, in JSON format.

```json
{
	"results": [
		{
			"path": "import/path/one"
		},
		{
			"path": "import/path/two"
		},
		{
			"path": "import/path/three"
		}
	]
}
```

**api.godoc.org/importers/`ImportPath`**&mdash;Returns packages that import ImportPath, in JSON format. Not recursive, direct imports only.

```json
{
	"results": [
		{
			"path": "import/path/one",
			"synopsis": "Package synopsis is here, if present."
		},
		{
			"path": "import/path/two",
			"synopsis": "Package synopsis is here, if present."
		}
	]
}
```

**api.godoc.org/imports/`ImportPath`**&mdash;Returns packages that ImportPath imports, in JSON format. Not recursive, direct imports only.

```json
{
	"imports": [
		{
			"path": "import/path/one",
			"synopsis": "Package synopsis is here, if present."
		},
		{
			"path": "import/path/two",
			"synopsis": "Package synopsis is here, if present."
		}
	],
	"testImports": [
		{
			"path": "import/path/three",
			"synopsis": "Package synopsis is here, if present."
		}
	]
}
```

A plain text interface is documented at <http://godoc.org/-/about>.
