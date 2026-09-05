// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/neko-sc/ent/examples/start/ent"
	"github.com/neko-sc/ent/examples/start/ent/car"
	"github.com/neko-sc/ent/examples/start/ent/group"
	"github.com/neko-sc/ent/examples/start/ent/user"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	client, err := ent.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	if err != nil {
		log.Fatalf("failed opening connection to sqlite: %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	// Run the auto migration tool.
	if err := client.Schema.Create(ctx); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}
	if _, err = CreateUser(ctx, client); err != nil {
		log.Fatal(err)
	}
	if _, err = QueryUser(ctx, client); err != nil {
		log.Fatal(err)
	}
	a8m, err := CreateCars(ctx, client)
	if err != nil {
		log.Fatal(err)
	}
	if err := QueryCars(ctx, a8m); err != nil {
		log.Fatal(err)
	}
	if err := QueryCarUsers(ctx, a8m); err != nil {
		log.Fatal(err)
	}
	if err := CreateGraph(ctx, client); err != nil {
		log.Fatal(err)
	}
	if err := QueryGithub(ctx, client); err != nil {
		log.Fatal(err)
	}
	if err := QueryArielCars(ctx, client); err != nil {
		log.Fatal(err)
	}
	if err := QueryGroupWithUsers(ctx, client); err != nil {
		log.Fatal(err)
	}
}

func CreateUser(ctx context.Context, client *ent.Client) (*ent.User, error) {
	u, err := client.User.
		Create().
		Set(user.Age, 30).
		Set(user.Name, "a8m").
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed creating user: %w", err)
	}
	log.Println("user was created: ", u)
	return u, nil
}

func QueryUser(ctx context.Context, client *ent.Client) (*ent.User, error) {
	u, err := client.User.
		Query().
		Where(user.Name.EQ("a8m")).
		// `Only` fails if no user found,
		// or more than 1 user returned.
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed querying user: %w", err)
	}
	log.Println("user returned: ", u)
	return u, nil
}

func CreateCars(ctx context.Context, client *ent.Client) (*ent.User, error) {
	// Create a new car with model "Tesla".
	tesla, err := client.Car.
		Create().
		Set(car.Model, "Tesla").
		Set(car.RegisteredAt, time.Now()).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed creating car: %w", err)
	}

	// Create a new car with model "Ford".
	ford, err := client.Car.
		Create().
		Set(car.Model, "Ford").
		Set(car.RegisteredAt, time.Now()).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed creating car: %w", err)
	}
	log.Println("car was created: ", ford)

	// Create a new user, and add it the 2 cars.
	a8m, err := client.User.
		Create().
		Set(user.Age, 30).
		Set(user.Name, "a8m").
		AddIDs(user.Cars, tesla.ID, ford.ID).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed creating user: %w", err)
	}
	log.Println("user was created: ", a8m)
	return a8m, nil
}

func QueryCars(ctx context.Context, a8m *ent.User) error {
	cars, err := a8m.QueryCars().All(ctx)
	if err != nil {
		return fmt.Errorf("failed querying user cars: %w", err)
	}
	log.Println("returned cars:", cars)

	// What about filtering specific cars.
	ford, err := a8m.QueryCars().
		Where(car.Model.EQ("Ford")).
		Only(ctx)
	if err != nil {
		return fmt.Errorf("failed querying user cars: %w", err)
	}
	log.Println(ford)
	return nil
}

func QueryCarUsers(ctx context.Context, a8m *ent.User) error {
	cars, err := a8m.QueryCars().All(ctx)
	if err != nil {
		return fmt.Errorf("failed querying user cars: %w", err)
	}

	// Query the inverse edge.
	for _, c := range cars {
		owner, err := c.QueryOwner().Only(ctx)
		if err != nil {
			return fmt.Errorf("failed querying car %q owner: %w", c.Model, err)
		}
		log.Printf("car %q owner: %q\n", c.Model, owner.Name)
	}
	return nil
}

func CreateGraph(ctx context.Context, client *ent.Client) error {
	// First, create the users.
	a8m, err := client.User.
		Create().
		Set(user.Age, 30).
		Set(user.Name, "Ariel").
		Save(ctx)
	if err != nil {
		return err
	}
	neta, err := client.User.
		Create().
		Set(user.Age, 28).
		Set(user.Name, "Neta").
		Save(ctx)
	if err != nil {
		return err
	}
	// Then, create the cars, and attach them to the users created above.
	err = client.Car.
		Create().
		Set(car.Model, "Tesla").
		Set(car.RegisteredAt, time.Now()).
		// Attach this car to Ariel.
		SetEdge(car.Owner, a8m.ID).
		Exec(ctx)
	if err != nil {
		return err
	}
	err = client.Car.
		Create().
		Set(car.Model, "Mazda").
		Set(car.RegisteredAt, time.Now()).
		// Attach this car to Ariel.
		SetEdge(car.Owner, a8m.ID).
		Exec(ctx)
	if err != nil {
		return err
	}
	err = client.Car.
		Create().
		Set(car.Model, "Ford").
		Set(car.RegisteredAt, time.Now()).
		// Attach this car to Neta.
		SetEdge(car.Owner, neta.ID).
		Exec(ctx)
	if err != nil {
		return err
	}
	// Create the groups, and add their users in the creation.
	err = client.Group.
		Create().
		Set(group.Name, "GitLab").
		AddIDs(group.Users, neta.ID, a8m.ID).
		Exec(ctx)
	if err != nil {
		return err
	}
	err = client.Group.
		Create().
		Set(group.Name, "GitHub").
		AddIDs(group.Users, a8m.ID).
		Exec(ctx)
	if err != nil {
		return err
	}
	log.Println("The graph was created successfully")
	return nil
}

func QueryGithub(ctx context.Context, client *ent.Client) error {
	cars, err := client.Group.
		Query().
		Where(group.Name.EQ("GitHub")). // (Group(Name=GitHub),)
		QueryUsers().                   // (User(Name=Ariel, Age=30),)
		QueryCars().                    // (Car(Model=Tesla, RegisteredAt=<Time>), Car(Model=Mazda, RegisteredAt=<Time>),)
		All(ctx)
	if err != nil {
		return fmt.Errorf("failed getting cars: %w", err)
	}
	log.Println("cars returned:", cars)
	// Output: (Car(Model=Tesla, RegisteredAt=<Time>), Car(Model=Mazda, RegisteredAt=<Time>),)
	return nil
}

func QueryArielCars(ctx context.Context, client *ent.Client) error {
	// Get "Ariel" from previous steps.
	a8m := client.User.
		Query().
		Where(
			user.Cars.Has(),
			user.Name.EQ("Ariel"),
		).
		OnlyX(ctx)
	cars, err := a8m. // Get the groups, that a8m is connected to:
				QueryGroups(). // (Group(Name=GitHub), Group(Name=GitLab),)
				QueryUsers().  // (User(Name=Ariel, Age=30), User(Name=Neta, Age=28),)
				QueryCars().   //
				Where(         //
			car.Not( //	Get Neta and Ariel cars, but filter out
				car.Model.EQ("Mazda"), //	those who named "Mazda"
			), //
		). //
		All(ctx)
	if err != nil {
		return fmt.Errorf("failed getting cars: %w", err)
	}
	log.Println("cars returned:", cars)
	return nil
}

func QueryGroupWithUsers(ctx context.Context, client *ent.Client) error {
	groups, err := client.Group.
		Query().
		Where(group.Users.Has()).
		All(ctx)
	if err != nil {
		return fmt.Errorf("failed getting groups: %w", err)
	}
	log.Println("groups returned:", groups)
	// Output: (Group(Name=GitHub), Group(Name=GitLab),)
	return nil
}
