// Copyright (c) 2012 The KidStuff Authors.
// Copyright (c) 2019 Andrey Shulepov.
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodbstore

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

var (
	ErrInvalidId = errors.New("mongodbstore: invalid session id")
)

// Session object store in MongoDB
type Session struct {
	ID       primitive.ObjectID `bson:"_id,omitempty"`
	Data     string
	Modified time.Time
}

// MongoDBStore stores sessions in MongoDB
type MongoDBStore struct {
	Codecs     []securecookie.Codec
	Options    *sessions.Options
	Token      TokenGetSeter
	collection *mongo.Collection
}

// NewMongoDBStore returns a new MongoDBStore.
// Set ensureTTL to true let the database auto-remove expired object by maxAge.
func NewMongoDBStore(c *mongo.Collection, maxAge int, ensureTTL bool, keyPairs ...[]byte) *MongoDBStore {
	store := &MongoDBStore{
		Codecs: securecookie.CodecsFromPairs(keyPairs...),
		Options: &sessions.Options{
			Path:   "/",
			MaxAge: maxAge,
		},
		Token:      &CookieToken{},
		collection: c,
	}

	store.MaxAge(maxAge)

	if ensureTTL {
		_, _ = c.Indexes().CreateOne(context.Background(), mongo.IndexModel{
			Keys: bsonx.Doc{{Key: "modified", Value: bsonx.Int32(1)}}, // value is the type 1 (asc) or -1 (desc)
			Options: &options.IndexOptions{
				Background:         newBool(true),
				Sparse:             newBool(true),
				ExpireAfterSeconds: newInt32(int32(maxAge)),
			},
		})
	}

	return store
}

// Get registers and returns a session for the given name and session store.
// It returns a new session if there are no sessions registered for the name.
func (m *MongoDBStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(m, name)
}

// New returns a session for the given name without adding it to the registry.
func (m *MongoDBStore) New(r *http.Request, name string) (*sessions.Session, error) {
	session := sessions.NewSession(m, name)
	session.Options = &sessions.Options{
		Path:     m.Options.Path,
		MaxAge:   m.Options.MaxAge,
		Domain:   m.Options.Domain,
		Secure:   m.Options.Secure,
		HttpOnly: m.Options.HttpOnly,
	}
	session.IsNew = true
	var err error
	if cook, errToken := m.Token.GetToken(r, name); errToken == nil {
		err = securecookie.DecodeMulti(name, cook, &session.ID, m.Codecs...)
		if err == nil {
			err = m.load(session)
			if err == nil {
				session.IsNew = false
			} else {
				err = nil
			}
		}
	}
	return session, err
}

// Save saves all sessions registered for the current request.
func (m *MongoDBStore) Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	if session.Options.MaxAge < 0 {
		if err := m.delete(session); err != nil {
			return err
		}
		m.Token.SetToken(w, session.Name(), "", session.Options)
		return nil
	}

	if session.ID == "" {
		session.ID = primitive.NewObjectID().Hex()
	}

	if err := m.upsert(session); err != nil {
		return err
	}

	encoded, err := securecookie.EncodeMulti(session.Name(), session.ID, m.Codecs...)
	if err != nil {
		return err
	}

	m.Token.SetToken(w, session.Name(), encoded, session.Options)
	return nil
}

// MaxAge sets the maximum age for the store and the underlying cookie
// implementation. Individual sessions can be deleted by setting Options.MaxAge
// = -1 for that session.
func (m *MongoDBStore) MaxAge(age int) {
	m.Options.MaxAge = age

	// Set the maxAge for each securecookie instance.
	for _, codec := range m.Codecs {
		if sc, ok := codec.(*securecookie.SecureCookie); ok {
			sc.MaxAge(age)
		}
	}
}

func (m *MongoDBStore) load(session *sessions.Session) error {
	sessionID, err := primitive.ObjectIDFromHex(session.ID)
	if err != nil {
		return ErrInvalidId
	}

	s := Session{}
	if err := m.collection.FindOne(context.Background(), bson.D{{"_id", sessionID}}).Decode(&s); err != nil {
		return err
	}

	if err := securecookie.DecodeMulti(session.Name(), s.Data, &session.Values, m.Codecs...); err != nil {
		return err
	}

	return nil
}

func (m *MongoDBStore) upsert(session *sessions.Session) error {
	sessionID, err := primitive.ObjectIDFromHex(session.ID)
	if err != nil {
		return ErrInvalidId
	}

	var modified time.Time
	if val, ok := session.Values["modified"]; ok {
		modified, ok = val.(time.Time)
		if !ok {
			return errors.New("mongodbstore: invalid modified value")
		}
	} else {
		modified = time.Now()
	}

	encoded, err := securecookie.EncodeMulti(session.Name(), session.Values, m.Codecs...)
	if err != nil {
		return err
	}

	s := Session{
		ID:       sessionID,
		Data:     encoded,
		Modified: modified,
	}

	_, err = m.collection.ReplaceOne(context.Background(), bson.D{{"_id", s.ID}}, &s, &options.ReplaceOptions{Upsert: newBool(true)})
	if err != nil {
		return err
	}

	return nil
}

func (m *MongoDBStore) delete(session *sessions.Session) error {
	sessionID, err := primitive.ObjectIDFromHex(session.ID)
	if err != nil {
		return ErrInvalidId
	}

	_, err = m.collection.DeleteOne(context.Background(), bson.D{{"_id", sessionID}})
	return err
}

func newBool(val bool) *bool {
	return &val
}

func newInt32(val int32) *int32 {
	return &val
}
