mongodbstore
==========

[Gorilla's Session](http://www.gorillatoolkit.org/pkg/sessions) store implementation with MongoDB

Forked from [github.com/kidstuff/mongostore](https://github.com/kidstuff/mongostore) to migrate from [mgo](https://labix.org/v2/mgo) to [mongo-go-driver](https://github.com/mongodb/mongo-go-driver).

## Requirements

Depends on the [mongo-go-driver](https://github.com/mongodb/mongo-go-driver) library.

## Installation

    go get github.com/ashulepov/mongodbstore

## Documentation

Available on [godoc.org](http://www.godoc.org/github.com/ashulepov/mongodbstore).

### Example
```go
    func foo(rw http.ResponseWriter, req *http.Request) {
        // Fetch new store.
        client, err := mongo.Connect(nil, options.Client().ApplyURI("mongodb://localhost:27017"))
        if err != nil {
        	panic(err)
        }
        defer client.Disconnect(nil)

        store := mongodbstore.NewMongoDBStore(client.Database("test").Collection("test_session"), 3600, true,
            []byte("secret-key"))

        // Get a session.
        session, err := store.Get(req, "session-key")
        if err != nil {
            log.Println(err.Error())
        }

        // Add a value.
        session.Values["foo"] = "bar"

        // Save.
        if err = sessions.Save(req, rw); err != nil {
            log.Printf("Error saving session: %v", err)
        }

        fmt.Fprintln(rw, "ok")
    }
```