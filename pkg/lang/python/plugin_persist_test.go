package python

import (
	"strings"
	"testing"

	"github.com/klothoplatform/klotho/pkg/annotation"
	"github.com/klothoplatform/klotho/pkg/compiler/types"
	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

// KV Tests
func Test_persister_queryKV(t *testing.T) {
	tests := []struct {
		name            string
		source          string
		matchName       string
		matchExpression string
	}{
		{
			name:            "cache constructor import match",
			source:          "from aiocache import Cache\nmyCache=Cache(Cache.MEMORY)",
			matchName:       "myCache",
			matchExpression: "myCache=Cache(Cache.MEMORY)",
		},
		{
			name:            "aiocache import match",
			source:          "import aiocache\nmyCache=aiocache.Cache(aiocache.Cache.MEMORY)",
			matchName:       "myCache",
			matchExpression: "myCache=aiocache.Cache(aiocache.Cache.MEMORY)",
		},
		{
			name: "aiocache import with alias",
			source: testutil.UnIndent(`
				import aiocache as a
				myCache=a.Cache(a.Cache.MEMORY)`),
			matchName:       "myCache",
			matchExpression: "myCache=a.Cache(a.Cache.MEMORY)",
		},
		{
			name:   "other 'Cache' function not matched",
			source: "import other\nmyCache=other.Cache(aiocache.Cache.MEMORY)\nmyCache=Cache(aiocache.Cache.MEMORY)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			f, err := NewFile("test.py", strings.NewReader(tt.source))
			if !assert.NoError(err) {
				return
			}

			cap := &types.Annotation{
				Capability: &annotation.Capability{Name: annotation.PersistCapability},
				Node:       f.Tree().RootNode(),
			}

			p := persister{}

			kvResult := p.queryKV(f, cap, true)

			if tt.matchExpression != "" || tt.matchName != "" {
				if assert.NotNil(kvResult) {
					assert.Equal(tt.matchExpression, kvResult.expression)
					assert.Equal(tt.matchName, kvResult.name)
				}
			} else {
				assert.Nil(kvResult)
			}
		})
	}
}

func Test_transformKV(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    string
		wantErr bool
	}{
		{
			name: "injects imports and updates cache constructor args",
			source: `
from aiocache import Cache
# @klotho::persist {
#   id="mycache"
# }
myCache = Cache(Cache.memory)`,
			want: `
from aiocache import Cache
import keyvalue
# @klotho::persist {
#   id="mycache"
# }
myCache = Cache(cache_class=keyvalue.KVStore, serializer=keyvalue.NoOpSerializer(), map_id="mycache")`,
		},
		{
			name: "overrides required args when initially set by the user",
			source: `
from aiocache import Cache
# @klotho::persist {
#   id="mycache"
# }
myCache = Cache(Cache.memory, serializer=MySerializer())`,
			want: `
from aiocache import Cache
import keyvalue
# @klotho::persist {
#   id="mycache"
# }
myCache = Cache(cache_class=keyvalue.KVStore, serializer=keyvalue.NoOpSerializer(), map_id="mycache")`,
		},
		{
			name: "leaves optional arguments provided by the user in place",
			source: `
from aiocache import Cache
# @klotho::persist {
#   id="mycache"
# }
myCache = Cache(Cache.memory, my_arg="value")`,
			want: `
from aiocache import Cache
import keyvalue
# @klotho::persist {
#   id="mycache"
# }
myCache = Cache(cache_class=keyvalue.KVStore, my_arg="value", serializer=keyvalue.NoOpSerializer(), map_id="mycache")`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			f, err := NewFile("test.py", strings.NewReader(tt.source))
			if !assert.NoError(err) {
				return
			}
			newF := f.CloneSourceFile()

			var cap *types.Annotation
			for _, v := range f.Annotations() {
				cap = v
				break
			}

			p := persister{
				runtime: NoopRuntime{},
			}

			ptype, pres := p.determinePersistType(f, cap)

			_, ok := ptype.(*types.Kv)
			if !assert.True(ok) {
				return
			}

			unit := &types.ExecutionUnit{}
			_, err = p.transformKV(f, newF, cap, pres, unit)
			if tt.wantErr {
				assert.Error(err)
				return
			}
			assert.NoError(err)
			assert.Equal(tt.want, string(newF.Program()))
		})
	}
}

// FS Tests
func Test_persister_queryFs(t *testing.T) {
	type result struct {
		matchName       string
		matchExpression string
	}
	tests := []struct {
		name    string
		imports []string
		// want is a slice of results, each corresponding to one top-level node in the source
		want []result
	}{
		{
			name:    "aiofiles import match",
			imports: []string{"import aiofiles"},
			want: []result{
				{
					matchName:       "aiofiles",
					matchExpression: "import aiofiles",
				},
			},
		},
		{
			name:    "aiofiles import alias match",
			imports: []string{"import aiofiles as fs"},
			want: []result{
				{
					matchName:       "fs",
					matchExpression: "import aiofiles as fs",
				},
			},
		},
		{
			name:    "other 'import not matched",
			imports: []string{"import other"},
			want:    []result{{}},
		},
		{
			name:    "imported with alias",
			imports: []string{`import aiofiles as fs`},
			want: []result{
				{
					matchName:       "fs",
					matchExpression: `import aiofiles as fs`,
				},
			},
		},
		{
			name: "imported twice with different aliases",
			imports: []string{
				`import aiofiles as first`,
				`import aiofiles as second`,
			},
			want: []result{
				{
					matchName:       "first",
					matchExpression: `import aiofiles as first`,
				},
				{
					matchName:       "second",
					matchExpression: `import aiofiles as second`,
				},
			},
		},
		{
			name:    "attribute import",
			imports: []string{`from aiofiles import open`},
			want: []result{
				{},
			},
		},
		{
			name:    "attribute import with alias",
			imports: []string{`from aiofiles import open as o`},
			want: []result{
				{},
			},
		},
		{
			name: "both supported and unsupported imports",
			imports: []string{
				`import aiofiles`,
				`from aiofiles import open`,
			},
			want: []result{
				{
					matchName:       "aiofiles",
					matchExpression: `import aiofiles`,
				},
				{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			f, err := NewFile("test.py", strings.NewReader(strings.Join(tt.imports, "\n")))
			if !assert.NoError(err) {
				return
			}

			for idx, imp := range tt.imports {
				childNode := testutil.FindNodeByContent(f.Tree().RootNode(), imp)
				want := tt.want[idx]

				cap := &types.Annotation{
					Capability: &annotation.Capability{Name: annotation.PersistCapability},
					Node:       childNode,
				}

				p := persister{}

				kvResult := p.queryFS(f, cap, true)

				if want.matchExpression != "" || want.matchName != "" {
					if assert.NotNil(kvResult) {
						assert.Equal(want.matchExpression, kvResult.expression)
						assert.Equal(want.matchName, kvResult.name)
					}
				} else {
					assert.Nilf(kvResult, "for item %d", idx)
				}
			}
		})
	}
}

func Test_transformFs(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    string
		wantErr bool
	}{
		{
			name: "injects imports as default",
			source: `
# @klotho::persist {
#   id="mycache"
# }
import aiofiles`,
			want: `
# @klotho::persist {
#   id="mycache"
# }
import klotho_runtime.fs_mycache as aiofiles`,
		},
		{
			name: "injects imports as alias",
			source: `
# @klotho::persist {
#   id="mycache"
# }
import aiofiles as fs`,
			want: `
# @klotho::persist {
#   id="mycache"
# }
import klotho_runtime.fs_mycache as fs`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			f, err := NewFile("test.py", strings.NewReader(tt.source))
			if !assert.NoError(err) {
				return
			}
			newF := f.CloneSourceFile()

			var cap *types.Annotation
			for _, v := range f.Annotations() {
				cap = v
				break
			}

			p := persister{
				runtime: NoopRuntime{},
			}

			ptype, pres := p.determinePersistType(f, cap)

			_, ok := ptype.(*types.Fs)
			if !assert.True(ok) {
				return
			}
			unit := &types.ExecutionUnit{}

			_, err = p.transformFS(f, newF, cap, pres, unit)
			if tt.wantErr {
				assert.Error(err)
				return
			}
			assert.NoError(err)
			assert.Equal(tt.want, string(newF.Program()))
		})
	}
}

// Secrets Tests
func Test_persister_querySecret(t *testing.T) {
	tests := []struct {
		name              string
		source            string
		secretsImportName string
		want              []string
		wantErr           bool
	}{
		{
			name: "get secret happy path",
			source: `
async with secrets.open('my_secret.key', mode='r') as f:
	result = await f.read()
	return result
			`,
			secretsImportName: "secrets",
			want:              []string{"my_secret.key"},
			wantErr:           false,
		},
		{
			name: "get secret happy path - multiple",
			source: `
async with secrets.open('my_secret.key', mode='r') as f:
	result = await f.read()
	return result
async with secrets.open('another_key.key', mode='r') as f:
	result = await f.read()
	return result
			`,
			secretsImportName: "secrets",
			want:              []string{"my_secret.key", "another_key.key"},
			wantErr:           false,
		},
		{
			name: "name mismatch continues",
			source: `
async with secrets.open('my_secret.key', mode='r') as f:
	result = await f.read()
	return result
			`,
			secretsImportName: "not_secrets",
			want:              []string{},
			wantErr:           false,
		},
		{
			name: "non read should error",
			source: `
async with secrets.open('my_secret.key', mode='r') as f:
	result = await f.write()
	return result
			`,
			secretsImportName: "secrets",
			want:              []string{},
			wantErr:           true,
		},
		{
			name: "non string as path should error",
			source: `
s = 'my_secret.key'
async with secrets.open(s, mode='r') as f:
	result = await f.read()
	return result
			`,
			secretsImportName: "secrets",
			want:              []string{},
			wantErr:           false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			f, err := NewFile("test.py", strings.NewReader(tt.source))
			if !assert.NoError(err) {
				return
			}

			p := persister{}

			secrets, err := p.querySecret(f, tt.secretsImportName)

			if tt.wantErr {
				assert.Error(err)
				return
			} else if !assert.NoError(err) {
				return
			}

			if len(tt.want) > 0 {
				assert.Equal(len(tt.want), len(secrets))
				for _, secret := range secrets {
					assert.Contains(tt.want, secret)
				}
			} else {
				assert.Empty(secrets)
			}
		})
	}
}

func Test_transformSecrets(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    string
		wantErr bool
	}{
		{
			name: "injects imports as default",
			source: `
# @klotho::persist {
#   id="mycache"
# }
import aiofiles`,
			want: `
# @klotho::persist {
#   id="mycache"
# }
import klotho_runtime.secret as aiofiles`,
		},
		{
			name: "injects imports as alias",
			source: `
# @klotho::persist {
#   id="mycache"
# }
import aiofiles as fs`,
			want: `
# @klotho::persist {
#   id="mycache"
# }
import klotho_runtime.secret as fs`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			f, err := NewFile("test.py", strings.NewReader(tt.source))
			if !assert.NoError(err) {
				return
			}
			newF := f.CloneSourceFile()

			var cap *types.Annotation
			for _, v := range f.Annotations() {
				cap = v
				break
			}

			p := persister{
				runtime: NoopRuntime{},
			}

			ptype, pres := p.determinePersistType(f, cap)

			_, ok := ptype.(*types.Fs)
			if !assert.True(ok) {
				return
			}

			unit := &types.ExecutionUnit{}

			_, err = p.transformSecret(f, newF, cap, pres, unit)
			if tt.wantErr {
				assert.Error(err)
				return
			}
			assert.NoError(err)
			assert.Equal(tt.want, string(newF.Program()))
		})
	}
}

// ORM Tests
func Test_persister_queryOrm(t *testing.T) {
	tests := []struct {
		name            string
		imports         string
		annotatedLine   string
		matchName       string
		matchExpression string
	}{
		{
			name:            "create engine constructor import match",
			imports:         `from sqlalchemy import create_engine`,
			annotatedLine:   `engine = create_engine("sqlite://", echo=True, future=True)`,
			matchName:       "engine",
			matchExpression: "\"sqlite://\"",
		},
		{
			name:            "sqlalchemy import match",
			imports:         `import sqlalchemy`,
			annotatedLine:   `engine = sqlalchemy.create_engine("sqlite://", echo=True, future=True)`,
			matchName:       "engine",
			matchExpression: "\"sqlite://\"",
		},
		{
			name:            "sqlalchemy import match as alias",
			imports:         `import sqlalchemy as sql`,
			annotatedLine:   `engine = sql.create_engine("sqlite://", echo=True, future=True)`,
			matchName:       "engine",
			matchExpression: "\"sqlite://\"",
		},
		{
			name:            "sqlalchemy import match as attribute alias",
			imports:         `from sqlalchemy import create_engine as eng`,
			annotatedLine:   `engine = eng("sqlite://", echo=True, future=True)`,
			matchName:       "engine",
			matchExpression: "\"sqlite://\"",
		},
		{
			name:            "create engine constructor import match only conn string",
			imports:         `from sqlalchemy import create_engine`,
			annotatedLine:   `engine = create_engine("sqlite://")`,
			matchName:       "engine",
			matchExpression: "\"sqlite://\"",
		},
		{
			name:          "create engine constructor import match no conn string",
			imports:       `from sqlalchemy import create_engine`,
			annotatedLine: `engine = create_engine()`,
		},
		{
			name:            "create engine constructor import match only conn string as fstring",
			imports:         `from sqlalchemy import create_engine`,
			annotatedLine:   `engine = create_engine(f'sqlite://{foo}')`,
			matchName:       "engine",
			matchExpression: "f'sqlite://{foo}'",
		},
		{
			name:            "create engine constructor import match only conn string as method",
			imports:         `from sqlalchemy import create_engine`,
			annotatedLine:   `engine = create_engine(method())`,
			matchName:       "engine",
			matchExpression: "method()",
		},
		{
			name: "other import function not matched",
			imports: testutil.UnIndent(`
				import other
				import sqlalchemy`),
			annotatedLine: `myCache = other.create_engine("sqlite://", echo=True, future=True)`,
		},
		{
			name: "other 'create_engine' function not matched",
			imports: testutil.UnIndent(`
				from other import create_engine
				from sqlalchemy import something`),
			annotatedLine: `myCache = other.create_engine("sqlite://", echo=True, future=True)`,
		},
		{
			name:            "import sqlalchemy with alias",
			imports:         `import sqlalchemy as s`,
			annotatedLine:   `engine = s.create_engine("sqlite://", echo=True, future=True)`,
			matchName:       "engine",
			matchExpression: "\"sqlite://\"",
		},
		{
			name:            "import sqlalchemy with alias",
			imports:         `import sqlalchemy as s`,
			annotatedLine:   `engine = s.create_engine("sqlite://", echo=True, future=True)`,
			matchName:       "engine",
			matchExpression: "\"sqlite://\"",
		},
		{
			name:            "import create_engine with alias",
			imports:         `from sqlalchemy import create_engine as ce`,
			annotatedLine:   `engine = ce(method())`,
			matchName:       "engine",
			matchExpression: "method()",
		},
		{
			name: "multiple imports with alias",
			imports: testutil.UnIndent(`
				import sqlalchemy
				from sqlalchemy import create_engine as something
				from sqlalchemy import create_engine as ce`),
			annotatedLine:   `engine = ce(method())`,
			matchName:       "engine",
			matchExpression: "method()",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			if !assert.NotContains(tt.annotatedLine, "\n", `bug in the test: "annotated"" line must be a single line`) {
				return
			}

			f, err := NewFile("test.py", strings.NewReader(tt.imports+"\n"+tt.annotatedLine))
			if !assert.NoError(err) {
				return
			}

			annotatedNode := testutil.FindNodeByContent(f.Tree().RootNode(), tt.annotatedLine)
			if !assert.NotNil(annotatedNode, "bug in the test: couldn't find annotated node") {
				return
			}

			cap := &types.Annotation{
				Capability: &annotation.Capability{Name: annotation.PersistCapability},
				Node:       annotatedNode,
			}

			p := persister{}

			ormResult := p.queryORM(f, cap, true)

			if tt.matchExpression != "" || tt.matchName != "" {
				if assert.NotNil(ormResult) {
					assert.Equal(tt.matchExpression, ormResult.expression)
					assert.Equal(tt.matchName, ormResult.name)
				}
			} else {
				assert.Nil(ormResult)
			}
		})
	}
}

func Test_transformOrm(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		expression string
		want       string
		wantErr    bool
	}{
		{
			name: "injects runtime from self import",
			source: `
import sqlalchemy
# @klotho::persist {
#   id = "sqlAlchemy"
# }
engine = sqlalchemy.create_engine("sqlite://", echo=True, future=True)`,
			expression: `"sqlite://"`,
			want: `
import sqlalchemy
import os
# @klotho::persist {
#   id = "sqlAlchemy"
# }
engine = sqlalchemy.create_engine(os.environ.get("SQLALCHEMY_PERSIST_ORM_CONNECTION"), echo=True, future=True)`,
		},
		{
			name: "injects runtime from named import",
			source: `
from sqlalchemy import create_engine
# @klotho::persist {
#   id = "sqlAlchemy"
# }
engine = create_engine("sqlite://", echo=True, future=True)`,
			expression: `"sqlite://"`,
			want: `
from sqlalchemy import create_engine
import os
# @klotho::persist {
#   id = "sqlAlchemy"
# }
engine = create_engine(os.environ.get("SQLALCHEMY_PERSIST_ORM_CONNECTION"), echo=True, future=True)`,
		},
		{
			name: "create engine constructor import match only conn string",
			source: `
from sqlalchemy import create_engine
# @klotho::persist {
#   id = "sqlAlchemy"
# }
engine = create_engine("sqlite://")`,
			expression: `"sqlite://"`,
			want: `
from sqlalchemy import create_engine
import os
# @klotho::persist {
#   id = "sqlAlchemy"
# }
engine = create_engine(os.environ.get("SQLALCHEMY_PERSIST_ORM_CONNECTION"))`,
		},
		{
			name: "create engine constructor import match only conn string as fstring",
			source: `
from sqlalchemy import create_engine
# @klotho::persist {
#   id = "sqlAlchemy"
# }
engine = create_engine(f'sqlite://{foo}')`,
			expression: `f'sqlite://{foo}'`,
			want: `
from sqlalchemy import create_engine
import os
# @klotho::persist {
#   id = "sqlAlchemy"
# }
engine = create_engine(os.environ.get("SQLALCHEMY_PERSIST_ORM_CONNECTION"))`,
		},
		{
			name: "create engine constructor import match only conn string as method",
			source: `
from sqlalchemy import create_engine
# @klotho::persist {
#   id = "sqlAlchemy"
# }
engine = create_engine(method())`,
			expression: `method()`,
			want: `
from sqlalchemy import create_engine
import os
# @klotho::persist {
#   id = "sqlAlchemy"
# }
engine = create_engine(os.environ.get("SQLALCHEMY_PERSIST_ORM_CONNECTION"))`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			f, err := NewFile("test.py", strings.NewReader(tt.source))
			if !assert.NoError(err) {
				return
			}
			newF := f.CloneSourceFile()

			var cap *types.Annotation
			for _, v := range f.Annotations() {
				cap = v
				break
			}

			p := persister{
				runtime: NoopRuntime{},
			}

			pres := &persistResult{
				name:       "engine",
				expression: tt.expression,
				construct:  &types.Orm{},
			}
			unit := &types.ExecutionUnit{}
			_, err = p.transformORM(f, newF, cap, pres, unit)
			if tt.wantErr {
				assert.Error(err)
				return
			}
			assert.NoError(err)
			assert.Equal(tt.want, string(newF.Program()))
			assert.Equal(types.EnvironmentVariables{
				{
					Name:      "SQLALCHEMY_PERSIST_ORM_CONNECTION",
					Construct: &types.Orm{Name: "sqlAlchemy"},
					Value:     "connection_string",
				},
			}, unit.EnvironmentVariables)
		})
	}
}

// redis Tests
func Test_persister_queryRedis(t *testing.T) {
	tests := []struct {
		name            string
		source          string
		matchName       string
		matchExpression string
		args            []FunctionArg
	}{
		{
			name: "create redis constructor import match",
			source: `
			from redis import Redis
			client = Redis(host='localhost', port=6379, db=0)
			`,
			matchName:       "client",
			matchExpression: "(host='localhost', port=6379, db=0)",
			args:            []FunctionArg{{Name: "host", Value: "'localhost'"}, {Name: "port", Value: "6379"}, {Name: "db", Value: "0"}},
		},
		{
			name: "redis import match",
			source: `
			import redis
			client = redis.Redis(host='localhost', port=6379, db=0)
			`,
			matchName:       "client",
			matchExpression: "(host='localhost', port=6379, db=0)",
			args:            []FunctionArg{{Name: "host", Value: "'localhost'"}, {Name: "port", Value: "6379"}, {Name: "db", Value: "0"}},
		},
		{
			name: "create redis constructor import match as alias",
			source: `
			from redis import Redis as r
			client = r(host='localhost', port=6379, db=0)
			`,
			matchName:       "client",
			matchExpression: "(host='localhost', port=6379, db=0)",
			args:            []FunctionArg{{Name: "host", Value: "'localhost'"}, {Name: "port", Value: "6379"}, {Name: "db", Value: "0"}},
		},
		{
			name: "redis import match as alias",
			source: `
			import redis as r
			client = r.Redis(host='localhost', port=6379, db=0)
			`,
			matchName:       "client",
			matchExpression: "(host='localhost', port=6379, db=0)",
			args:            []FunctionArg{{Name: "host", Value: "'localhost'"}, {Name: "port", Value: "6379"}, {Name: "db", Value: "0"}},
		},
		{
			name: "other import function not matched",
			source: `
			import other
			import redis
			client = other.Redis(host='localhost', port=6379, db=0)
			`,
		},
		{
			name: "other 'redis' function not matched",
			source: `
			from other import Redis
			from sqlalchemy import something
			client = Redis(host='localhost', port=6379, db=0)
			`,
		},
		{
			name: "RedisCluster imported self matched",
			source: `
			import redis
			client = redis.cluster.RedisCluster(host='localhost', port=6379)
			`,
			matchName:       "client",
			matchExpression: "(host='localhost', port=6379)",
			args:            []FunctionArg{{Name: "host", Value: "'localhost'"}, {Name: "port", Value: "6379"}},
		},
		{
			name: "RedisCluster import cluster from redis matched",
			source: `
			from redis import cluster
			client = cluster.RedisCluster(host='localhost', port=6379)
			`,
			matchName:       "client",
			matchExpression: "(host='localhost', port=6379)",
			args:            []FunctionArg{{Name: "host", Value: "'localhost'"}, {Name: "port", Value: "6379"}},
		},
		{
			name: "RedisCluster import from redis.cluster matched",
			source: `
			from redis.cluster import RedisCluster
			client = RedisCluster(host='localhost', port=6379)
			`,
			matchName:       "client",
			matchExpression: "(host='localhost', port=6379)",
			args:            []FunctionArg{{Name: "host", Value: "'localhost'"}, {Name: "port", Value: "6379"}},
		},
		{
			name: "RedisCluster imported self matched as alias",
			source: `
			import redis as r
			client = r.cluster.RedisCluster(host='localhost', port=6379)
			`,
			matchName:       "client",
			matchExpression: "(host='localhost', port=6379)",
			args:            []FunctionArg{{Name: "host", Value: "'localhost'"}, {Name: "port", Value: "6379"}},
		},
		{
			name: "RedisCluster import cluster from redis matched as alias",
			source: `
			from redis import cluster as c
			client = c.RedisCluster(host='localhost', port=6379)
			`,
			matchName:       "client",
			matchExpression: "(host='localhost', port=6379)",
			args:            []FunctionArg{{Name: "host", Value: "'localhost'"}, {Name: "port", Value: "6379"}},
		},
		{
			name: "RedisCluster import from redis.cluster matched as alias",
			source: `
			from redis.cluster import RedisCluster as rc
			client = rc(host='localhost', port=6379)
			`,
			matchName:       "client",
			matchExpression: "(host='localhost', port=6379)",
			args:            []FunctionArg{{Name: "host", Value: "'localhost'"}, {Name: "port", Value: "6379"}},
		},
		{
			name: "RedisCluster imported self matched as alias no match",
			source: `
			import redis as r
			client = redis.cluster.RedisCluster(host='localhost', port=6379, db=0)
			`,
		},
		{
			name: "RedisCluster import cluster from redis matched as alias no match",
			source: `
			from redis import cluster as c
			client = cluster.RedisCluster(host='localhost', port=6379, db=0)
			`,
		},
		{
			name: "RedisCluster import from redis.cluster matched as alias no match",
			source: `
			from redis.cluster import RedisCluster as rc
			client = RedisCluster(host='localhost', port=6379, db=0)
			`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			f, err := NewFile("test.py", strings.NewReader(tt.source))
			if !assert.NoError(err) {
				return
			}

			cap := &types.Annotation{
				Capability: &annotation.Capability{Name: annotation.PersistCapability, ID: "redis"},
				Node:       f.Tree().RootNode(),
			}

			p := persister{
				runtime: NoopRuntime{},
			}

			result := p.queryRedis(f, cap, true)

			if tt.matchExpression != "" || tt.matchName != "" {
				if assert.NotNil(result) {
					assert.Equal(tt.matchExpression, result.expression)
					assert.Equal(tt.matchName, result.name)
					assert.Equal(tt.args, result.args)
				}
			} else {
				assert.Nil(result)
			}
		})
	}
}

func Test_transformRedis(t *testing.T) {
	tests := []struct {
		name           string
		source         string
		redisConstruct construct.Construct
		want           string
		wantErr        bool
	}{
		{
			name: "injects runtime from self import",
			source: `
import redis
# @klotho::persist {
#   id = "redis"
# }
client = redis.Redis(host='localhost', port=6379)
`,
			redisConstruct: &types.RedisNode{Name: "redis"},
			want: `
import redis
import os
# @klotho::persist {
#   id = "redis"
# }
client = redis.Redis(host=os.environ.get("REDIS_PERSIST_REDIS_HOST"), port=os.environ.get("REDIS_PERSIST_REDIS_PORT"))
`,
		},
		{
			name: "injects runtime from named import",
			source: `
from redis import Redis
# @klotho::persist {
#   id = "redis"
# }
client = Redis(host='localhost', port=6379)`,
			redisConstruct: &types.RedisNode{Name: "redis"},
			want: `
from redis import Redis
import os
# @klotho::persist {
#   id = "redis"
# }
client = Redis(host=os.environ.get("REDIS_PERSIST_REDIS_HOST"), port=os.environ.get("REDIS_PERSIST_REDIS_PORT"))`,
		},
		{
			name: "injects cluster runtime from self import",
			source: `
import redis
# @klotho::persist {
#   id = "redis"
# }
client = redis.cluster.RedisCluster(host='localhost', port=6379)
`,
			redisConstruct: &types.RedisCluster{Name: "redis"},
			want: `
import redis
import os
# @klotho::persist {
#   id = "redis"
# }
client = redis.cluster.RedisCluster(host=os.environ.get("REDIS_PERSIST_REDIS_HOST"), port=os.environ.get("REDIS_PERSIST_REDIS_PORT"), ssl=True, skip_full_coverage_check=True)
`,
		},
		{
			name: "injects cluster runtime from named import",
			source: `
from redis import cluster
# @klotho::persist {
#   id = "redis"
# }
client = cluster.RedisCluster(host='localhost', port=6379)`,
			redisConstruct: &types.RedisCluster{Name: "redis"},
			want: `
from redis import cluster
import os
# @klotho::persist {
#   id = "redis"
# }
client = cluster.RedisCluster(host=os.environ.get("REDIS_PERSIST_REDIS_HOST"), port=os.environ.get("REDIS_PERSIST_REDIS_PORT"), ssl=True, skip_full_coverage_check=True)`,
		},
		{
			name: "injects RedisCluster runtime from named import",
			source: `
from redis.cluster import RedisCluster
# @klotho::persist {
#   id = "redis"
# }
client = RedisCluster(host='localhost', port=6379)`,
			redisConstruct: &types.RedisCluster{Name: "redis"},
			want: `
from redis.cluster import RedisCluster
import os
# @klotho::persist {
#   id = "redis"
# }
client = RedisCluster(host=os.environ.get("REDIS_PERSIST_REDIS_HOST"), port=os.environ.get("REDIS_PERSIST_REDIS_PORT"), ssl=True, skip_full_coverage_check=True)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			f, err := NewFile("test.py", strings.NewReader(tt.source))
			if !assert.NoError(err) {
				return
			}
			newF := f.CloneSourceFile()

			var cap *types.Annotation
			for _, v := range f.Annotations() {
				cap = v
				break
			}

			p := persister{
				runtime: NoopRuntime{},
			}

			pres := &persistResult{
				name:       "client",
				expression: "(host='localhost', port=6379)",
				args:       []FunctionArg{{Name: "host", Value: "localhost"}, {Name: "port", Value: "6379"}},
				construct:  tt.redisConstruct,
			}

			unit := &types.ExecutionUnit{}
			_, err = p.transformRedis(f, newF, cap, pres, unit)
			if tt.wantErr {
				assert.Error(err)
				return
			}
			assert.NoError(err)
			assert.Equal(tt.want, string(newF.Program()))

			assert.Equal(types.EnvironmentVariables{
				{
					Name:      "REDIS_PERSIST_REDIS_HOST",
					Construct: tt.redisConstruct,
					Value:     "host",
				},
				{
					Name:      "REDIS_PERSIST_REDIS_PORT",
					Construct: tt.redisConstruct,
					Value:     "port",
				},
			}, unit.EnvironmentVariables)
		})
	}
}
