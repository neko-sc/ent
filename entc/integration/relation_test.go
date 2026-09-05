// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/neko-sc/ent/entc/integration/ent"
	"github.com/neko-sc/ent/entc/integration/ent/card"
	"github.com/neko-sc/ent/entc/integration/ent/group"
	groupinfo "github.com/neko-sc/ent/entc/integration/ent/groupinfo"
	"github.com/neko-sc/ent/entc/integration/ent/node"
	"github.com/neko-sc/ent/entc/integration/ent/pet"
	"github.com/neko-sc/ent/entc/integration/ent/user"

	"github.com/stretchr/testify/require"
)

// Demonstrate a O2O relation between two different types. A User and a CreditCard.
// The user is the owner of the edge, named "owner", and the card has an inverse edge
// named "owner" that points to the User.
func O2OTwoTypes(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without card")
	usr := client.User.Create().Set(user.Age, 10).Set(user.Name, "foo").SaveX(ctx)
	require.Zero(usr.QueryCard().CountX(ctx))

	t.Log("add card to user on card creation (inverse creation)")
	crd := client.Card.Create().Set(card.Number, "1").SetEdge(card.Owner, usr.ID).SaveX(ctx)
	require.Equal(usr.QueryCard().CountX(ctx), 1)
	require.Equal(crd.QueryOwner().CountX(ctx), 1)

	t.Log("delete inverse should delete association")
	client.Card.DeleteOne(crd).ExecX(ctx)
	require.Zero(client.Card.Query().CountX(ctx))
	require.Zero(usr.QueryCard().CountX(ctx), "user should not have card")

	t.Log("add card to user by updating user (the owner of the edge)")
	crd = client.Card.Create().Set(card.Number, "10").SaveX(ctx)
	usr.Update().SetEdge(user.Card, crd.ID).ExecX(ctx)
	require.Equal(usr.Name, crd.QueryOwner().OnlyX(ctx).Name)
	require.Equal(crd.Number, usr.QueryCard().OnlyX(ctx).Number)

	t.Log("delete assoc should delete inverse edge")
	client.User.DeleteOne(usr).ExecX(ctx)
	require.Zero(client.User.Query().CountX(ctx))
	require.Zero(crd.QueryOwner().CountX(ctx), "card should not have an owner")

	t.Log("add card to user by updating card (the inverse edge)")
	usr = client.User.Create().Set(user.Age, 10).Set(user.Name, "bar").SaveX(ctx)
	crd.Update().SetEdge(card.Owner, usr.ID).ExecX(ctx)
	require.Equal(usr.Name, crd.QueryOwner().OnlyX(ctx).Name)
	require.Equal(crd.Number, usr.QueryCard().OnlyX(ctx).Number)

	t.Log("query with side lookup on inverse")
	ocrd := client.Card.Create().Set(card.Number, "orphan card").SaveX(ctx)
	require.Equal(crd.Number, client.Card.Query().Where(card.Owner.Has()).OnlyX(ctx).Number)
	require.Equal(ocrd.Number, client.Card.Query().Where(card.Not(card.Owner.Has())).OnlyX(ctx).Number)

	t.Log("query with side lookup on assoc")
	ousr := client.User.Create().Set(user.Age, 10).Set(user.Name, "user without card").SaveX(ctx)
	require.Equal(usr.Name, client.User.Query().Where(user.Card.Has()).OnlyX(ctx).Name)
	require.Equal(ousr.Name, client.User.Query().Where(user.Not(user.Card.Has())).OnlyX(ctx).Name)

	t.Log("query with side lookup condition on inverse")
	require.Equal(crd.Number, client.Card.Query().Where(card.Owner.HasWith(user.Name.EQ(usr.Name))).OnlyX(ctx).Number)
	// has owner, but with name != "bar".
	require.Zero(client.Card.Query().Where(card.Owner.HasWith(user.Not(user.Name.EQ(usr.Name)))).CountX(ctx))
	// either has no owner, or has owner with name != "bar".
	require.Equal(
		ocrd.Number,
		client.Card.Query().
			Where(
				card.Or(
					// has no owner.
					card.Not(card.Owner.Has()),
					// has owner with name != "bar".
					card.Owner.HasWith(user.Not(user.Name.EQ(usr.Name))),
				),
			).
			OnlyX(ctx).Number,
	)

	t.Log("query with side lookup condition on assoc")
	require.Equal(usr.Name, client.User.Query().Where(user.Card.HasWith(card.Number.EQ(crd.Number))).OnlyX(ctx).Name)
	require.Zero(client.User.Query().Where(user.Card.HasWith(card.Not(card.Number.EQ(crd.Number)))).CountX(ctx))
	// either has no card, or has card with number != "10".
	require.Equal(
		ousr.Name,
		client.User.Query().
			Where(
				user.Or(
					// has no card.
					user.Not(user.Card.Has()),
					// has card with number != "10".
					user.Card.HasWith(card.Not(card.Number.EQ(crd.Number))),
				),
			).
			OnlyX(ctx).Name,
	)

	t.Log("query long path from inverse")
	require.Equal(crd.Number, crd.QueryOwner().QueryCard().OnlyX(ctx).Number, "should get itself")
	require.Equal(usr.Name, crd.QueryOwner().QueryCard().QueryOwner().OnlyX(ctx).Name, "should get its owner")
	require.Equal(
		usr.Name,
		crd.QueryOwner().
			Where(user.Card.Has()).
			QueryCard().
			QueryOwner().
			Where(user.Card.Has()).
			OnlyX(ctx).Name,
		"should get its owner",
	)

	t.Log("query long path from assoc")
	require.Equal(usr.Name, usr.QueryCard().QueryOwner().OnlyX(ctx).Name, "should get itself")
	require.Equal(crd.Number, usr.QueryCard().QueryOwner().QueryCard().OnlyX(ctx).Number, "should get its card")
	require.Equal(
		crd.Number,
		usr.QueryCard().
			Where(card.Owner.Has()).
			QueryOwner().
			Where(user.Card.Has()).
			QueryCard().
			OnlyX(ctx).Number,
		"should get its card",
	)
}

// Demonstrate a O2O relation between two instances of the same type. A linked-list
// nodes, where each node has an edge named "next" with inverse named "prev".
func O2OSameType(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("head of the list")
	head := client.Node.Create().Set(node.Value, 1).SaveX(ctx)
	require.Zero(head.QueryPrev().CountX(ctx))
	require.Zero(head.QueryNext().CountX(ctx))

	t.Log("add node to the linked-list and connect it to the head (inverse creation)")
	sec := client.Node.Create().Set(node.Value, 2).SetEdge(node.Prev, head.ID).SaveX(ctx)
	require.Zero(sec.QueryNext().CountX(ctx), "should not have next")
	require.Equal(head.ID, sec.QueryPrev().OnlyX(ctx).ID, "head should point to the second node")
	require.Equal(sec.ID, head.QueryNext().OnlyX(ctx).ID)
	require.Equal(2, client.Node.Query().CountX(ctx), "linked-list should have 2 nodes")

	t.Log("delete inverse should delete association")
	client.Node.DeleteOne(sec).ExecX(ctx)
	require.Zero(head.QueryNext().CountX(ctx))
	require.Equal(1, client.Node.Query().CountX(ctx), "linked-list should have 1 node")

	t.Log("add node to the linked-list by updating the head (the owner of the edge)")
	sec = client.Node.Create().Set(node.Value, 2).SaveX(ctx)
	head.Update().SetEdge(node.Next, sec.ID).ExecX(ctx)
	require.Zero(sec.QueryNext().CountX(ctx), "should not have next")
	require.Equal(head.ID, sec.QueryPrev().OnlyX(ctx).ID, "head should point to the second node")
	require.Equal(sec.ID, head.QueryNext().OnlyX(ctx).ID)
	require.Equal(2, client.Node.Query().CountX(ctx), "linked-list should have 2 nodes")

	t.Log("delete assoc should delete inverse edge")
	client.Node.DeleteOne(head).ExecX(ctx)
	require.Zero(sec.QueryPrev().CountX(ctx), "second node should be the head now")
	require.Zero(sec.QueryNext().CountX(ctx), "second node should be the head now")

	t.Log("update second node value to be 1")
	head = sec.Update().Set(node.Value, 1).SaveX(ctx)
	require.Equal(1, head.Value)

	t.Log("create a linked-list 1->2->3->4->5")
	nodes := []*ent.Node{head}
	for i := 0; i < 4; i++ {
		next := client.Node.Create().Set(node.Value, nodes[i].Value+1).SetEdge(node.Prev, nodes[i].ID).SaveX(ctx)
		nodes = append(nodes, next)
	}
	require.Equal(len(nodes), client.Node.Query().CountX(ctx))

	t.Log("check correctness of the list values")
	for i, n := range nodes[:3] {
		require.Equal(i+1, n.Value)
		require.Equal(nodes[i+1].Value, n.QueryNext().OnlyX(ctx).Value)
	}
	require.Zero(nodes[len(nodes)-1].QueryNext().CountX(ctx), "last node should point to nil")

	t.Log("query with side lookup on inverse/assoc")
	require.Equal(4, client.Node.Query().Where(node.Next.Has()).CountX(ctx))
	require.Equal(4, client.Node.Query().Where(node.Prev.Has()).CountX(ctx))

	t.Log("make the linked-list to be circular")
	nodes[len(nodes)-1].Update().SetEdge(node.Next, head.ID).SaveX(ctx)
	require.Equal(nodes[0].Value, nodes[len(nodes)-1].QueryNext().OnlyX(ctx).Value, "last node should point to head")
	require.Equal(nodes[len(nodes)-1].Value, nodes[0].QueryPrev().OnlyX(ctx).Value, "head should have a reference to the tail")

	t.Log("query with side lookup on inverse/assoc")
	require.Equal(5, client.Node.Query().Where(node.Next.Has()).CountX(ctx))
	require.Equal(5, client.Node.Query().Where(node.Prev.Has()).CountX(ctx))
	// node that points (with "next") to other node with value 2 (the head).
	require.Equal(nodes[0].Value, client.Node.Query().Where(node.Next.HasWith(node.Value.EQ(2))).OnlyX(ctx).Value)
	// node that points (with "next") to other node with value 1 (the tail).
	require.Equal(nodes[len(nodes)-1].Value, client.Node.Query().Where(node.Next.HasWith(node.Value.EQ(1))).OnlyX(ctx).Value)
	// nodes that points to nodes with value greater than 2 (X->2->3->4->X).
	values, err := ent.Values(ctx, client.Node.Query().
		Where(node.Next.HasWith(node.Value.GT(2))).
		Order(node.Value.Asc()).
		GroupBy(node.Value), node.Value)
	require.NoError(err)
	require.Equal([]int{2, 3, 4}, values)

	t.Log("query long path from inverse")
	// going back from head to tail until we reach the head.
	require.Equal(
		head.Value,
		head.
			QueryPrev(). // 5 (tail)
			QueryPrev(). // 4
			QueryPrev(). // 3
			QueryPrev(). // 2
			QueryPrev(). // 1 (head)
			OnlyX(ctx).Value,
	)
	// disrupt the query in the middle.
	require.Zero(head.QueryPrev().QueryPrev().Where(node.Value.GT(10)).QueryPrev().QueryPrev().QueryPrev().CountX(ctx))

	t.Log("query long path from assoc")
	// going forward from head to next until we reach the head.
	require.Equal(
		head.Value,
		head.
			QueryNext(). // 2
			QueryNext(). // 3
			QueryNext(). // 4
			QueryNext(). // 5 (tail)
			QueryNext(). // 1 (head)
			OnlyX(ctx).Value,
	)
	// disrupt the query in the middle.
	require.Zero(head.QueryNext().QueryNext().Where(node.Value.GT(10)).QueryNext().QueryNext().QueryNext().CountX(ctx))

	t.Log("delete all nodes except the head")
	client.Node.Delete().Where(node.Value.GT(1)).ExecX(ctx)
	head = client.Node.Query().OnlyX(ctx)

	t.Log("node points to itself (circular linked-list with 1 node)")
	head.Update().SetEdge(node.Next, head.ID).SaveX(ctx)
	require.Equal(head.ID, head.QueryPrev().OnlyIDX(ctx))
	require.Equal(head.ID, head.QueryNext().OnlyIDX(ctx))
	head.Update().ClearEdge(node.Next).SaveX(ctx)
	require.Zero(head.QueryPrev().CountX(ctx))
	require.Zero(head.QueryNext().CountX(ctx))
}

// Demonstrate a O2O relation between two instances of the same type, where the relation
// has the same name in both directions. A couple. User A has "spouse" B (and vice versa).
// When setting B as a spouse of A, this sets A as spouse of B as well. In other words:
//
//	foo := client.User.Create().SetName("foo").SaveX(ctx)
//	bar := client.User.Create().SetName("bar").SetSpouse(foo).SaveX(ctx)
//	count := client.User.Query.Where(user.Spouse.Has()).CountX(ctx)
//	// count will be 2, even though we've created only one relation above.
func O2OSelfRef(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without spouse")
	foo := client.User.Create().Set(user.Age, 10).Set(user.Name, "foo").SaveX(ctx)
	require.False(foo.QuerySpouse().ExistX(ctx))

	t.Log("sets spouse on user creation (inverse creation)")
	bar := client.User.Create().Set(user.Age, 10).Set(user.Name, "bar").SetEdge(user.Spouse, foo.ID).SaveX(ctx)
	require.True(foo.QuerySpouse().ExistX(ctx))
	require.True(bar.QuerySpouse().ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.Spouse.Has()).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(bar).ExecX(ctx)
	require.False(foo.QuerySpouse().ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Spouse.Has()).CountX(ctx))

	t.Log("add spouse to user by updating a user")
	bar = client.User.Create().Set(user.Age, 10).Set(user.Name, "bar").SaveX(ctx)
	foo.Update().SetEdge(user.Spouse, bar.ID).ExecX(ctx)
	require.True(foo.QuerySpouse().ExistX(ctx))
	require.True(bar.QuerySpouse().ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.Spouse.Has()).CountX(ctx))

	t.Log("remove a spouse using update")
	foo.Update().ClearEdge(user.Spouse).ExecX(ctx)
	require.False(foo.QuerySpouse().ExistX(ctx))
	require.False(bar.QuerySpouse().ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Spouse.Has()).CountX(ctx))
	// return back the spouse.
	foo.Update().SetEdge(user.Spouse, bar.ID).ExecX(ctx)

	t.Log("create a user without spouse")
	baz := client.User.Create().Set(user.Age, 10).Set(user.Name, "baz").SaveX(ctx)
	require.False(baz.QuerySpouse().ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.Spouse.Has()).CountX(ctx))

	t.Log("set a new spouse")
	foo.Update().ClearEdge(user.Spouse).SetEdge(user.Spouse, baz.ID).ExecX(ctx)
	require.True(foo.QuerySpouse().ExistX(ctx))
	require.True(baz.QuerySpouse().ExistX(ctx))
	require.False(bar.QuerySpouse().ExistX(ctx))
	// return back the spouse.
	foo.Update().ClearEdge(user.Spouse).SetEdge(user.Spouse, bar.ID).ExecX(ctx)

	t.Log("spouse is a unique edge")
	require.Error(baz.Update().SetEdge(user.Spouse, bar.ID).Exec(ctx))
	require.Error(baz.Update().SetEdge(user.Spouse, foo.ID).Exec(ctx))

	t.Log("query with side lookup")
	require.Equal(
		bar.Name,
		client.User.Query().
			Where(user.Spouse.HasWith(user.Name.EQ("foo"))).
			OnlyX(ctx).Name,
	)
	require.Equal(
		foo.Name,
		client.User.Query().
			Where(user.Spouse.HasWith(user.Name.EQ("bar"))).
			OnlyX(ctx).Name,
	)
	require.Equal(
		baz.Name,
		client.User.Query().
			Where(user.Not(user.Spouse.Has())).
			OnlyX(ctx).Name,
	)
	// has spouse that has a spouse with name "foo" (which actually means itself).
	require.Equal(
		foo.Name,
		client.User.Query().
			Where(user.Spouse.HasWith(user.Spouse.HasWith(user.Name.EQ("foo")))).
			OnlyX(ctx).Name,
	)
	// has spouse that has a spouse with name "bar" (which actually means itself).
	require.Equal(
		bar.Name,
		client.User.Query().
			Where(user.Spouse.HasWith(user.Spouse.HasWith(user.Name.EQ("bar")))).
			OnlyX(ctx).Name,
	)

	t.Log("query path from a user")
	require.Equal(
		foo.Name,
		foo.
			QuerySpouse(). // bar
			QuerySpouse(). // foo
			QuerySpouse(). // bar
			QuerySpouse(). // foo
			OnlyX(ctx).Name,
	)
	require.Equal(
		bar.Name,
		bar.
			QuerySpouse(). // foo
			QuerySpouse(). // bar
			QuerySpouse(). // foo
			QuerySpouse(). // bar
			OnlyX(ctx).Name,
	)

	t.Log("query path from client")
	require.Equal(
		bar.Name,
		client.User.
			Query().
			Where(user.Name.EQ("foo")). // foo
			QuerySpouse().              // bar
			OnlyX(ctx).Name,
	)
	require.Equal(
		bar.Name,
		client.User.
			Query().
			Where(user.Name.EQ("bar")). // bar
			QuerySpouse().              // foo
			QuerySpouse().              // bar
			OnlyX(ctx).Name,
	)
}

// Demonstrate a O2M/M2O relation between two different types. A User and its Pets.
// The User type is the "owner" of the edge (assoc), and the Pet as an inverse edge to
// its owner. User can have one or more Pets, and Pet have only one owner (not required).
func O2MTwoTypes(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without pet")
	usr := client.User.Create().Set(user.Age, 30).Set(user.Name, "a8m").SaveX(ctx)
	require.False(usr.QueryPets().ExistX(ctx))

	t.Log("add pet to user on pet creation (inverse creation)")
	pedro := client.Pet.Create().Set(pet.Name, "pedro").SetEdge(pet.Owner, usr.ID).SaveX(ctx)
	require.Equal(usr.Name, pedro.QueryOwner().OnlyX(ctx).Name)
	require.Equal(pedro.Name, usr.QueryPets().OnlyX(ctx).Name)

	t.Log("delete inverse should delete association")
	client.Pet.DeleteOne(pedro).ExecX(ctx)
	require.Zero(client.Pet.Query().CountX(ctx))
	require.False(usr.QueryPets().ExistX(ctx), "user should not have pet")

	t.Log("add pet to user by updating user (the owner of the edge)")
	pedro = client.Pet.Create().Set(pet.Name, "pedro").SaveX(ctx)
	usr.Update().AddIDs(user.Pets, pedro.ID).ExecX(ctx)
	require.Equal(usr.Name, pedro.QueryOwner().OnlyX(ctx).Name)
	require.Equal(pedro.Name, usr.QueryPets().OnlyX(ctx).Name)

	t.Log("delete assoc (owner of the edge) should delete inverse edge")
	client.User.DeleteOne(usr).ExecX(ctx)
	require.Zero(client.User.Query().CountX(ctx))
	require.False(pedro.QueryOwner().ExistX(ctx), "pet should not have an owner")

	t.Log("add pet to user by updating pet (the inverse edge)")
	usr = client.User.Create().Set(user.Age, 30).Set(user.Name, "a8m").SaveX(ctx)
	pedro.Update().SetEdge(pet.Owner, usr.ID).ExecX(ctx)
	require.Equal(usr.Name, pedro.QueryOwner().OnlyX(ctx).Name)
	require.Equal(pedro.Name, usr.QueryPets().OnlyX(ctx).Name)

	t.Log("add another pet to user")
	xabi := client.Pet.Create().Set(pet.Name, "xabi").SetEdge(pet.Owner, usr.ID).SaveX(ctx)
	require.Equal(2, usr.QueryPets().CountX(ctx))
	require.Equal(1, xabi.QueryOwner().CountX(ctx))
	require.Equal(1, pedro.QueryOwner().CountX(ctx))

	t.Log("edge is unique on the inverse side")
	_, err := client.User.Create().Set(user.Age, 30).Set(user.Name, "alex").AddIDs(user.Pets, pedro.ID).Save(ctx)
	require.Error(err, "pet already has an owner")

	t.Log("add multiple pets on creation")
	p1 := client.Pet.Create().Set(pet.Name, "p1").SaveX(ctx)
	p2 := client.Pet.Create().Set(pet.Name, "p2").SaveX(ctx)
	usr2 := client.User.Create().Set(user.Age, 30).Set(user.Name, "alex").AddIDs(user.Pets, p1.ID, p2.ID).SaveX(ctx)
	require.True(p1.QueryOwner().ExistX(ctx))
	require.True(p2.QueryOwner().ExistX(ctx))
	require.Equal(2, usr2.QueryPets().CountX(ctx))
	// delete p1, p2.
	client.Pet.Delete().Where(pet.ID.In(p1.ID, p2.ID)).ExecX(ctx)
	require.Zero(usr2.QueryPets().CountX(ctx))

	t.Log("change the owner a pet")
	xabi.Update().ClearEdge(pet.Owner).SetEdge(pet.Owner, usr2.ID).ExecX(ctx)
	require.Equal(1, usr.QueryPets().CountX(ctx))
	require.Equal(1, usr2.QueryPets().CountX(ctx))
	require.Equal(usr2.Name, xabi.QueryOwner().OnlyX(ctx).Name)

	t.Log("query with side lookup on inverse")
	opet := client.Pet.Create().Set(pet.Name, "orphan pet").SaveX(ctx)
	require.Equal(opet.Name, client.Pet.Query().Where(pet.Not(pet.Owner.Has())).OnlyX(ctx).Name)
	require.Equal(2, client.Pet.Query().Where(pet.Owner.Has()).CountX(ctx))

	t.Log("query with side lookup on assoc")
	require.Zero(client.User.Query().Where(user.Not(user.Pets.Has())).CountX(ctx))
	ousr := client.User.Create().Set(user.Age, 10).Set(user.Name, "user without pet").SaveX(ctx)
	require.Equal(2, client.User.Query().Where(user.Pets.Has()).CountX(ctx))
	require.Equal(ousr.Name, client.User.Query().Where(user.Not(user.Pets.Has())).OnlyX(ctx).Name)

	t.Log("query with side lookup condition on inverse")
	require.Equal(pedro.Name, client.Pet.Query().Where(pet.Owner.HasWith(user.Name.EQ(usr.Name))).OnlyX(ctx).Name)
	// has owner, but with name != "a8m".
	require.Equal(xabi.Name, client.Pet.Query().Where(pet.Owner.HasWith(user.Not(user.Name.EQ(usr.Name)))).OnlyX(ctx).Name)
	// either has no owner, or has owner with name != "alex" and name != "a8m".
	require.Equal(
		opet.Name,
		client.Pet.Query().
			Where(
				pet.Or(
					// has no owner.
					pet.Not(pet.Owner.Has()),
					// has owner with name != "a8m" and name != "alex".
					pet.Owner.HasWith(
						user.Not(user.Name.EQ(usr.Name)),
						user.Not(user.Name.EQ(usr2.Name)),
					),
				),
			).
			OnlyX(ctx).Name,
	)

	t.Log("query with side lookup condition on assoc")
	require.Equal(usr.Name, client.User.Query().Where(user.Pets.HasWith(pet.Name.EQ(pedro.Name))).OnlyX(ctx).Name)
	require.Equal(usr2.Name, client.User.Query().Where(user.Pets.HasWith(pet.Name.EQ(xabi.Name))).OnlyX(ctx).Name)
	require.Zero(
		client.User.Query().
			Where(
				user.Pets.HasWith(
					pet.Not(pet.Name.EQ(xabi.Name)),
					pet.Not(pet.Name.EQ(pedro.Name)),
				),
			).CountX(ctx),
	)
	// either has no pet, or has pet with name != "pedro" and name != "xabi".
	require.Equal(
		ousr.Name,
		client.User.Query().
			Where(
				user.Or(
					// has no pet.
					user.Not(user.Pets.Has()),
					// has pet with name != "pedro" and name != "xabi".
					user.Pets.HasWith(
						pet.Not(pet.Name.EQ(xabi.Name)),
						pet.Not(pet.Name.EQ(pedro.Name)),
					),
				),
			).
			OnlyX(ctx).Name,
	)

	t.Log("query long path from inverse")
	require.Equal(pedro.Name, pedro.QueryOwner().QueryPets().OnlyX(ctx).Name, "should get itself")
	require.Equal(usr.Name, pedro.QueryOwner().QueryPets().QueryOwner().OnlyX(ctx).Name, "should get its owner")
	require.Equal(
		usr.Name,
		pedro.QueryOwner().
			Where(user.Pets.Has()).
			QueryPets().
			QueryOwner().
			Where(user.Pets.Has()).
			OnlyX(ctx).Name,
		"should get its owner",
	)

	t.Log("query long path from assoc")
	require.Equal(usr.Name, usr.QueryPets().QueryOwner().OnlyX(ctx).Name, "should get itself")
	require.Equal(pedro.Name, usr.QueryPets().QueryOwner().QueryPets().OnlyX(ctx).Name, "should get its pet")
	require.Equal(
		pedro.Name,
		usr.QueryPets().
			Where(pet.Owner.Has()). // pedro
			QueryOwner().           //
			Where(user.Pets.Has()). // a8m
			QueryPets().            // pedro
			OnlyX(ctx).Name,
		"should get its pet",
	)
	require.Equal(
		xabi.Name,
		client.User.Query().
			// alex matches this query (not a8m, and have a pet).
			Where(
				user.Not(user.Name.EQ(usr.Name)),
				user.Pets.Has(),
			).
			QueryPets().  // xabi
			QueryOwner(). // alex
			QueryPets().  // xabi
			OnlyX(ctx).Name,
	)
}

// Demonstrate a O2M/M2O relation between two instances of the same type. A "parent" and
// its children. User can have one or more children, but can have only one parent (unique inverse edge).
// Note that both edges are not required.
func O2MSameType(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new parent without children")
	prt := client.User.Create().Set(user.Age, 30).Set(user.Name, "a8m").SaveX(ctx)
	require.Zero(prt.QueryChildren().CountX(ctx))

	t.Log("add child to parent on child creation (inverse creation)")
	chd := client.User.Create().Set(user.Age, 1).Set(user.Name, "child").SetEdge(user.Parent, prt.ID).SaveX(ctx)
	require.Equal(prt.Name, chd.QueryParent().OnlyX(ctx).Name)
	require.Equal(chd.Name, prt.QueryChildren().OnlyX(ctx).Name)

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(chd).ExecX(ctx)
	require.False(prt.QueryChildren().ExistX(ctx), "user should not have children")

	t.Log("add child to parent by updating user (the owner of the edge)")
	chd = client.User.Create().Set(user.Age, 1).Set(user.Name, "child").SaveX(ctx)
	prt.Update().AddIDs(user.Children, chd.ID).ExecX(ctx)
	require.Equal(prt.Name, chd.QueryParent().OnlyX(ctx).Name)
	require.Equal(chd.Name, prt.QueryChildren().OnlyX(ctx).Name)

	t.Log("delete assoc (owner of the edge) should delete inverse edge")
	client.User.DeleteOne(prt).ExecX(ctx)
	require.Equal(1, client.User.Query().CountX(ctx))
	require.False(chd.QueryParent().ExistX(ctx), "child should not have an owner")

	t.Log("add pet to user by updating pet (the inverse edge)")
	prt = client.User.Create().Set(user.Age, 30).Set(user.Name, "a8m").SaveX(ctx)
	chd.Update().SetEdge(user.Parent, prt.ID).ExecX(ctx)
	require.Equal(prt.Name, chd.QueryParent().OnlyX(ctx).Name)
	require.Equal(chd.Name, prt.QueryChildren().OnlyX(ctx).Name)
	require.Zero(prt.QueryParent().CountX(ctx), "parent is orphan")
	require.Zero(chd.QueryChildren().CountX(ctx), "child should not have children")

	t.Log("add another pet to user")
	chd2 := client.User.Create().Set(user.Age, 1).Set(user.Name, "child2").SetEdge(user.Parent, prt.ID).SaveX(ctx)
	require.Equal(2, prt.QueryChildren().CountX(ctx))
	require.Equal(1, chd.QueryParent().CountX(ctx))
	require.Equal(1, chd2.QueryParent().CountX(ctx))

	t.Log("edge is unique on the inverse side")
	_, err := client.User.Create().Set(user.Age, 30).Set(user.Name, "alex").AddIDs(user.Children, chd.ID).Save(ctx)
	require.Error(err, "child already has parent")
	_, err = client.User.Create().Set(user.Age, 30).Set(user.Name, "alex").AddIDs(user.Children, chd2.ID).Save(ctx)
	require.Error(err, "child already has parent")

	t.Log("add multiple child on creation")
	chd3 := client.User.Create().Set(user.Age, 1).Set(user.Name, "child3").SaveX(ctx)
	chd4 := client.User.Create().Set(user.Age, 1).Set(user.Name, "child4").SaveX(ctx)
	prt2 := client.User.Create().Set(user.Age, 30).Set(user.Name, "alex").AddIDs(user.Children, chd3.ID, chd4.ID).SaveX(ctx)
	require.True(chd3.QueryParent().ExistX(ctx))
	require.True(chd3.QueryParent().ExistX(ctx))
	require.Equal(2, prt2.QueryChildren().CountX(ctx))
	// delete chd3, chd4.
	client.User.Delete().Where(user.ID.In(chd3.ID, chd4.ID)).ExecX(ctx)
	require.Zero(prt2.QueryChildren().CountX(ctx))

	t.Log("change the parent a child")
	chd2.Update().ClearEdge(user.Parent).SetEdge(user.Parent, prt2.ID).ExecX(ctx)
	require.Equal(1, prt.QueryChildren().CountX(ctx))
	require.Equal(1, prt2.QueryChildren().CountX(ctx))
	require.Equal(chd2.Name, prt2.QueryChildren().OnlyX(ctx).Name)

	t.Log("query with side lookup on inverse")
	ochd := client.User.Create().Set(user.Age, 1).Set(user.Name, "orphan user").SaveX(ctx)
	require.Equal(3, client.User.Query().Where(user.Not(user.Parent.Has())).CountX(ctx))
	require.Equal(
		ochd.Name,
		client.User.Query().
			Where(
				user.Not(user.Parent.Has()),
				user.Not(user.Children.Has()),
			).
			OnlyX(ctx).Name,
		"3 orphan users, but only one does not have children",
	)
	require.Equal(2, client.User.Query().Where(user.Parent.Has()).CountX(ctx))

	t.Log("query with side lookup on assoc")
	require.Equal(2, client.User.Query().Where(user.Children.Has()).CountX(ctx))
	require.Equal(3, client.User.Query().Where(user.Not(user.Children.Has())).CountX(ctx))

	t.Log("query with side lookup condition on inverse")
	require.Equal(chd.Name, client.User.Query().Where(user.Parent.HasWith(user.Name.EQ(prt.Name))).OnlyX(ctx).Name)
	// has parent, but with name != "a8m".
	require.Equal(chd2.Name, client.User.Query().Where(user.Parent.HasWith(user.Not(user.Name.EQ(prt.Name)))).OnlyX(ctx).Name)
	// either has no parent, or has parent with name != "alex".
	require.Equal(
		4,
		client.User.Query().
			Where(
				user.Or(
					// has no parent.
					user.Not(user.Parent.Has()),
					// has parent with name != "alex".
					user.Parent.HasWith(
						user.Not(user.Name.EQ(prt2.Name)),
					),
				),
			).
			CountX(ctx),
		"should match chd, ochd, prt, prt2",
	)
	// either has no parent, or has parent with name != "a8m".
	require.Equal(
		4,
		client.User.Query().
			Where(
				user.Or(
					// has no parent.
					user.Not(user.Parent.Has()),
					// has parent with name != "a8m".
					user.Parent.HasWith(
						user.Not(user.Name.EQ(prt.Name)),
					),
				),
			).
			CountX(ctx),
		"should match chd2, ochd, prt, prt2",
	)

	t.Log("query with side lookup condition on assoc")
	require.Equal(prt.Name, client.User.Query().Where(user.Children.HasWith(user.Name.EQ(chd.Name))).OnlyX(ctx).Name)
	require.Equal(prt2.Name, client.User.Query().Where(user.Children.HasWith(user.Name.EQ(chd2.Name))).OnlyX(ctx).Name)
	// parent with 2 children named: child and child2.
	require.Zero(
		client.User.Query().
			Where(
				user.Children.HasWith(
					user.Name.EQ(chd.Name),
					user.Name.EQ(chd2.Name),
				),
			).
			CountX(ctx),
	)
	// either has no children, or has 2 children: "child" and "child2".
	require.Equal(
		3,
		client.User.Query().
			Where(
				user.Or(
					// has no children.
					user.Not(user.Children.Has()),
					// has 2 children: "child" and "child2".
					user.Children.HasWith(
						user.Name.EQ(chd.Name),
						user.Name.EQ(chd2.Name),
					),
				),
			).
			CountX(ctx),
		"should match chd, chd2 and ochd",
	)

	t.Log("query long path from inverse")
	require.Equal(chd.Name, chd.QueryParent().QueryChildren().OnlyX(ctx).Name, "should get itself")
	require.Equal(prt.Name, chd.QueryParent().QueryChildren().QueryParent().OnlyX(ctx).Name, "should get its parent")
	require.Equal(
		prt.Name,
		chd.QueryParent().
			Where(user.Children.Has()).
			QueryChildren().
			QueryParent().
			Where(user.Children.Has()).
			OnlyX(ctx).Name,
		"should get its owner",
	)

	t.Log("query long path from assoc")
	require.Equal(prt.Name, prt.QueryChildren().QueryParent().OnlyX(ctx).Name, "should get itself")
	require.Equal(chd.Name, prt.QueryChildren().QueryParent().QueryChildren().OnlyX(ctx).Name, "should get its child")
	require.Equal(
		chd.Name,
		prt.QueryChildren().
			Where(user.Parent.Has()).   // child
			QueryParent().              //
			Where(user.Children.Has()). // parent
			QueryChildren().            // child
			OnlyX(ctx).Name,
		"should get its child",
	)
	require.Equal(
		chd2.Name,
		client.User.Query().
			// "alex" matches this query (not "a8m", and have a child).
			Where(
				user.Not(user.Name.EQ(prt.Name)),
				user.Children.Has(),
			).
			QueryChildren(). // child
			QueryParent().   // parent
			QueryChildren(). // child
			OnlyX(ctx).Name,
	)
}

// Demonstrate a M2M relation between two instances of the same type, where the relation
// has the same name in both directions. A friendship between Users.
// User A has "friend" B (and vice versa). When setting B as a friend of A, this sets A
// as friend of B as well. In other words:
//
//	foo := client.User.Create().SetName("foo").SaveX(ctx)
//	bar := client.User.Create().SetName("bar").AddFriends(foo).SaveX(ctx)
//	count := client.User.Query.Where(user.Friends.Has()).CountX(ctx)
//	// count will be 2, even though we've created only one relation above.
func M2MSelfRef(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without friends")
	foo := client.User.Create().Set(user.Age, 10).Set(user.Name, "foo").SaveX(ctx)
	require.False(foo.QueryFriends().ExistX(ctx))

	t.Log("sets friendship on user creation (inverse creation)")
	bar := client.User.Create().Set(user.Age, 10).Set(user.Name, "bar").AddIDs(user.Friends, foo.ID).SaveX(ctx)
	require.True(foo.QueryFriends().ExistX(ctx))
	require.True(bar.QueryFriends().ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.Friends.Has()).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(bar).ExecX(ctx)
	require.False(foo.QueryFriends().ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Friends.Has()).CountX(ctx))

	t.Log("add friendship to user by updating existing users")
	bar = client.User.Create().Set(user.Age, 10).Set(user.Name, "bar").SaveX(ctx)
	foo.Update().AddIDs(user.Friends, bar.ID).ExecX(ctx)
	require.True(foo.QueryFriends().ExistX(ctx))
	require.True(bar.QueryFriends().ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.Friends.Has()).CountX(ctx))

	t.Log("remove friendship using update")
	foo.Update().RemoveIDs(user.Friends, bar.ID).ExecX(ctx)
	require.False(foo.QueryFriends().ExistX(ctx))
	require.False(bar.QueryFriends().ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Friends.Has()).CountX(ctx))
	// return back the friendship.
	foo.Update().AddIDs(user.Friends, bar.ID).ExecX(ctx)

	t.Log("create a user without friends")
	baz := client.User.Create().Set(user.Age, 10).Set(user.Name, "baz").SaveX(ctx)
	require.False(baz.QueryFriends().ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.Friends.Has()).CountX(ctx))

	t.Log("both baz and bar are friends of foo")
	baz.Update().AddIDs(user.Friends, foo.ID).ExecX(ctx)
	require.Equal(2, foo.QueryFriends().CountX(ctx))
	require.Equal(foo.Name, bar.QueryFriends().OnlyX(ctx).Name)
	require.Equal(foo.Name, baz.QueryFriends().OnlyX(ctx).Name)
	require.Equal(3, client.User.Query().Where(user.Friends.Has()).CountX(ctx))

	t.Log("query with side lookup")
	friendNames, projectionError := ent.Values(ctx, client.User.Query().
		Where(user.Friends.HasWith(user.Name.EQ(foo.Name))).
		Order(user.Name.Asc()).
		GroupBy(user.Name), user.Name)
	require.NoError(projectionError)
	require.Equal(
		[]string{bar.Name, baz.Name},
		friendNames,
	)
	require.Equal(
		foo.Name,
		client.User.Query().
			Where(user.Friends.HasWith(user.Name.EQ(bar.Name))).
			OnlyX(ctx).Name,
	)
	require.Equal(
		foo.Name,
		client.User.Query().
			Where(user.Not(user.Friends.HasWith(user.Name.EQ(foo.Name)))).
			OnlyX(ctx).Name,
		"foo does not have friendship with foo",
	)
	nonfriendNames, projectionError := ent.Values(ctx, client.User.Query().
		Where(user.Not(user.Friends.HasWith(user.Name.EQ(baz.Name)))).
		Order(user.Name.Asc()).
		GroupBy(user.Name), user.Name)
	require.NoError(projectionError)
	require.Equal(
		[]string{bar.Name, baz.Name},
		nonfriendNames,
		"bar and baz do not have friendship with baz",
	)

	t.Log("query path from a user")
	require.Equal(
		foo.Name,
		foo.
			QueryFriends().Where(user.Name.EQ(bar.Name)). // bar
			QueryFriends().                               // foo
			QueryFriends().Where(user.Name.EQ(baz.Name)). // baz
			QueryFriends().                               // foo
			OnlyX(ctx).Name,
	)
	require.Equal(
		foo.Name,
		foo.
			QueryFriends(). // bar, baz
			QueryFriends(). // foo
			QueryFriends(). // bar, baz
			QueryFriends(). // foo
			OnlyX(ctx).Name,
	)
	require.Equal(
		baz.Name,
		foo.
			QueryFriends().Where(user.Name.EQ(bar.Name)).           // bar
			QueryFriends().                                         // foo
			QueryFriends().Where(user.Not(user.Name.EQ(bar.Name))). // baz
			OnlyX(ctx).Name,
	)

	t.Log("query path from client")
	traversedNames, projectionError := ent.Values(ctx, client.User.
		Query().
		Where(user.Name.EQ(foo.Name)). // foo
		QueryFriends().                // bar, baz
		Order(user.Name.Asc()).
		GroupBy(user.Name), user.Name)
	require.NoError(projectionError)
	require.Equal(
		[]string{bar.Name, baz.Name},
		traversedNames,
	)
	require.Equal(
		bar.Name,
		client.User.
			Query().
			// foo has a friend (bar) that does not have a friend named baz.
			Where(
				user.Friends.HasWith(
					user.Not(
						user.Friends.HasWith(user.Name.EQ(baz.Name)),
					),
				),
			).
			// bar and baz.
			QueryFriends().
			// filter baz out.
			Where(user.Not(user.Name.EQ(baz.Name))).
			OnlyX(ctx).Name,
	)
}

// Demonstrate a M2M relation between two instances of the same type.
// Following and followers.
func M2MSameType(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without followers")
	foo := client.User.Create().Set(user.Age, 10).Set(user.Name, "foo").SaveX(ctx)
	require.False(foo.QueryFollowers().ExistX(ctx))

	t.Log("adds followers on user creation (inverse creation)")
	bar := client.User.Create().Set(user.Age, 10).Set(user.Name, "bar").AddIDs(user.Following, foo.ID).SaveX(ctx)
	require.Equal(foo.Name, bar.QueryFollowing().OnlyX(ctx).Name)
	require.Equal(bar.Name, foo.QueryFollowers().OnlyX(ctx).Name)
	require.Equal(1, client.User.Query().Where(user.Followers.Has()).CountX(ctx))
	require.Equal(1, client.User.Query().Where(user.Following.Has()).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(bar).ExecX(ctx)
	require.False(foo.QueryFollowers().ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Followers.Has()).CountX(ctx))
	require.Zero(client.User.Query().Where(user.Following.Has()).CountX(ctx))

	t.Log("add followers to user by updating existing users")
	bar = client.User.Create().Set(user.Age, 10).Set(user.Name, "bar").SaveX(ctx)
	foo.Update().AddIDs(user.Followers, bar.ID).ExecX(ctx)
	require.Equal(foo.Name, bar.QueryFollowing().OnlyX(ctx).Name)
	require.Equal(bar.Name, foo.QueryFollowers().OnlyX(ctx).Name)
	require.Equal(1, client.User.Query().Where(user.Followers.Has()).CountX(ctx))
	require.Equal(1, client.User.Query().Where(user.Following.Has()).CountX(ctx))

	t.Log("remove following using update")
	bar.Update().RemoveIDs(user.Following, foo.ID).ExecX(ctx)
	require.False(foo.QueryFollowers().ExistX(ctx))
	require.False(bar.QueryFollowing().ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Following.Has()).CountX(ctx))
	require.Zero(client.User.Query().Where(user.Followers.Has()).CountX(ctx))
	// follow back.
	bar.Update().AddIDs(user.Following, foo.ID).ExecX(ctx)

	t.Log("remove followers using update (inverse)")
	foo.Update().RemoveIDs(user.Followers, bar.ID).ExecX(ctx)
	require.False(foo.QueryFollowers().ExistX(ctx))
	require.False(bar.QueryFollowing().ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Following.Has()).CountX(ctx))
	require.Zero(client.User.Query().Where(user.Followers.Has()).CountX(ctx))
	// follow back.
	bar.Update().AddIDs(user.Following, foo.ID).ExecX(ctx)

	users := make([]*ent.User, 5)
	for i := range users {
		u := client.User.Create().Set(user.Age, 10).Set(user.Name, fmt.Sprintf("user-%d", i)).SaveX(ctx)
		users[i] = u.Update().AddIDs(user.Following, foo.ID, bar.ID).SaveX(ctx)
		followingNames, projectionError := ent.Values(ctx, u.QueryFollowing().
			Order(user.Name.Asc()).
			GroupBy(user.Name), user.Name)
		require.NoError(projectionError)
		require.Equal(
			[]string{bar.Name, foo.Name},
			followingNames,
		)
	}
	require.Equal(5, bar.QueryFollowers().CountX(ctx), "users1..5")
	require.Equal(6, foo.QueryFollowers().CountX(ctx), "users1..5 and bar")
	require.Equal(2, client.User.Query().Where(user.Followers.Has()).CountX(ctx), "foo and bar")
	require.Equal(6, client.User.Query().Where(user.Following.Has()).CountX(ctx), "users1..5 and bar")
	// compare followers.
	fooFollowers, projectionError := ent.Values(ctx, foo.QueryFollowers().
		Where(user.Not(user.Name.EQ(bar.Name))).
		Order(user.Name.Asc()).
		GroupBy(user.Name), user.Name)
	require.NoError(projectionError)
	barFollowers, projectionError := ent.Values(ctx, bar.QueryFollowers().
		Order(user.Name.Asc()).
		GroupBy(user.Name), user.Name)
	require.NoError(projectionError)
	require.Equal(
		barFollowers,
		fooFollowers,
		"bar.followers = (foo.followers - bar)",
	)

	// delete users 1..5.
	client.User.Delete().Where(user.Name.HasPrefix("user")).ExecX(ctx)
	require.Equal(2, client.User.Query().CountX(ctx))

	t.Log("query with side lookup from inverse")
	require.Equal(foo.Name, foo.QueryFollowers().QueryFollowing().OnlyX(ctx).Name, "should get itself")
	require.Equal(bar.Name, foo.QueryFollowers().QueryFollowing().QueryFollowers().OnlyX(ctx).Name, "should get its follower (bar)")

	t.Log("query with side lookup from assoc")
	require.Equal(bar.Name, bar.QueryFollowing().QueryFollowers().OnlyX(ctx).Name, "should get itself")
	require.Equal(foo.Name, bar.QueryFollowing().QueryFollowers().QueryFollowing().OnlyX(ctx).Name, "should get foo")

	// generate additional users and make sure we don't get them in the queries below.
	client.User.Create().Set(user.Age, 10).Set(user.Name, "baz").SaveX(ctx)
	client.User.Create().Set(user.Age, 10).Set(user.Name, "qux").SaveX(ctx)

	t.Log("query path from a user")
	require.Equal(
		bar.Name,
		foo.
			QueryFollowers().Where(user.Name.EQ(bar.Name)). // bar
			QueryFollowing().Where(user.Followers.Has()).   // foo
			QueryFollowers().                               // bar
			Where(
				user.Following.HasWith(
					user.Name.EQ(foo.Name),
				),
			).
			OnlyX(ctx).Name,
	)

	t.Log("query path from client")
	require.Equal(
		foo.Name,
		client.User.
			Query().Where(user.Name.EQ(foo.Name)).          // foo
			QueryFollowers().Where(user.Name.EQ(bar.Name)). // bar
			QueryFollowing().Where(user.Followers.Has()).   // foo
			QueryFollowers().                               // bar
			Where(
				user.Following.HasWith(
					user.Name.EQ(foo.Name),
				),
			).
			// has followers named bar (foo).
			QueryFollowing().
			Where(
				user.Followers.HasWith(
					user.Name.EQ(bar.Name),
				),
			).
			OnlyX(ctx).Name,
	)
}

// Demonstrate a M2M relation between two different types. User and groups.
func M2MTwoTypes(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without groups")
	foo := client.User.Create().Set(user.Age, 10).Set(user.Name, "foo").SaveX(ctx)
	require.False(foo.QueryGroups().ExistX(ctx))
	require.Zero(client.Group.Query().CountX(ctx))

	t.Log("adds users to group on group creation (inverse creation)")
	// group-info is required edge.
	inf := client.GroupInfo.Create().Set(groupinfo.Desc, "desc").SaveX(ctx)
	hub := client.Group.Create().Set(group.Name, "Github").Set(group.Expire, time.Now()).AddIDs(group.Users, foo.ID).SetEdge(group.Info, inf.ID).SaveX(ctx)
	require.Equal(foo.Name, hub.QueryUsers().OnlyX(ctx).Name, "group has only one user")
	require.Equal(hub.Name, foo.QueryGroups().OnlyX(ctx).Name, "user is connected to one group")
	require.Equal(1, client.User.Query().Where(user.Groups.Has()).CountX(ctx))
	require.Equal(1, client.Group.Query().Where(group.Users.Has()).CountX(ctx))

	t.Log("add an existing M2M edge should not throw an error")
	foo.Update().AddIDs(user.Groups, hub.ID).ExecX(ctx)
	require.Equal(1, foo.QueryGroups().CountX(ctx))
	hub.Update().AddIDs(group.Users, foo.ID).ExecX(ctx)
	require.Equal(1, hub.QueryUsers().CountX(ctx))

	t.Log("delete inverse should delete association")
	client.Group.DeleteOne(hub).ExecX(ctx)
	require.False(foo.QueryGroups().ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Groups.Has()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.Users.Has()).CountX(ctx))

	t.Log("add user to groups updating existing users")
	hub = client.Group.Create().Set(group.Name, "Github").Set(group.Expire, time.Now()).SetEdge(group.Info, inf.ID).SaveX(ctx)
	require.False(foo.QueryGroups().ExistX(ctx))
	foo.Update().AddIDs(user.Groups, hub.ID).ExecX(ctx)
	require.Equal(foo.Name, hub.QueryUsers().OnlyX(ctx).Name, "group has only one user")
	require.Equal(hub.Name, foo.QueryGroups().OnlyX(ctx).Name, "user is connected to one group")
	require.Equal(1, client.User.Query().Where(user.Groups.Has()).CountX(ctx))
	require.Equal(1, client.Group.Query().Where(group.Users.Has()).CountX(ctx))

	t.Log("delete assoc should delete inverse as well")
	client.User.DeleteOne(foo).ExecX(ctx)
	require.False(hub.QueryUsers().ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Groups.Has()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.Users.Has()).CountX(ctx))
	// add back the user.
	foo = client.User.Create().Set(user.Age, 10).Set(user.Name, "foo").AddIDs(user.Groups, hub.ID).SaveX(ctx)

	t.Log("remove following using update (assoc)")
	foo.Update().RemoveIDs(user.Groups, hub.ID).ExecX(ctx)
	require.False(foo.QueryGroups().ExistX(ctx))
	require.False(hub.QueryUsers().ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Groups.Has()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.Users.Has()).CountX(ctx))
	// join back to group.
	foo.Update().AddIDs(user.Groups, hub.ID).ExecX(ctx)

	t.Log("remove following using update (inverse)")
	hub.Update().RemoveIDs(group.Users, foo.ID).ExecX(ctx)
	require.False(foo.QueryGroups().ExistX(ctx))
	require.False(hub.QueryUsers().ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Groups.Has()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.Users.Has()).CountX(ctx))
	// add back the user.
	hub.Update().AddIDs(group.Users, foo.ID).ExecX(ctx)

	t.Log("multiple groups and users")
	lab := client.Group.Create().Set(group.Name, "Gitlab").Set(group.Expire, time.Now()).SetEdge(group.Info, inf.ID).SaveX(ctx)
	bar := client.User.Create().Set(user.Age, 10).Set(user.Name, "bar").SaveX(ctx)
	require.Equal(1, client.User.Query().Where(user.Groups.Has()).CountX(ctx))
	require.Equal(1, client.Group.Query().Where(group.Users.Has()).CountX(ctx))
	bar.Update().AddIDs(user.Groups, lab.ID).ExecX(ctx)
	require.Equal(2, client.User.Query().Where(user.Groups.Has()).CountX(ctx))
	require.Equal(2, client.Group.Query().Where(group.Users.Has()).CountX(ctx))
	// validate relations.
	require.Equal(foo.Name, hub.QueryUsers().OnlyX(ctx).Name, "hub has only one user")
	require.Equal(hub.Name, foo.QueryGroups().OnlyX(ctx).Name, "foo is connected only to hub")
	require.Equal(bar.Name, lab.QueryUsers().OnlyX(ctx).Name, "lab has only one user")
	require.Equal(lab.Name, bar.QueryGroups().OnlyX(ctx).Name, "bar is connected only to lab")
	// add bar to hub.
	bar.Update().AddIDs(user.Groups, hub.ID).ExecX(ctx)
	require.Equal(2, hub.QueryUsers().CountX(ctx))
	require.Equal(1, lab.QueryUsers().CountX(ctx))
	userNames, projectionError := ent.Values(ctx, hub.QueryUsers().Order(user.Name.Asc()).GroupBy(user.Name), user.Name)
	require.NoError(projectionError)
	require.Equal([]string{bar.Name, foo.Name}, userNames)
	groupNames, projectionError := ent.Values(ctx, bar.QueryGroups().Order(group.Name.Asc()).GroupBy(group.Name), group.Name)
	require.NoError(projectionError)
	require.Equal([]string{hub.Name, lab.Name}, groupNames)

	t.Log("query with side lookup from inverse")
	require.Equal(hub.Name, hub.QueryUsers().QueryGroups().Where(group.Name.EQ(hub.Name)).OnlyX(ctx).Name, "should get itself")
	require.Equal(bar.Name, lab.QueryUsers().QueryGroups().Where(group.Not(group.Name.EQ(hub.Name))).QueryUsers().OnlyX(ctx).Name, "should get its user")

	t.Log("query with side lookup from assoc")
	require.Equal(bar.Name, bar.QueryGroups().Where(group.Name.EQ(lab.Name)).QueryUsers().OnlyX(ctx).Name, "should get itself")
	require.Equal(lab.Name, bar.QueryGroups().Where(group.Name.EQ(lab.Name)).QueryUsers().QueryGroups().Where(group.Name.EQ(lab.Name)).OnlyX(ctx).Name, "should get its group")

	t.Log("query path from a user")
	require.Equal(
		hub.Name,
		bar.
			// hub.
			QueryGroups().
			Where(
				group.Users.HasWith(user.Name.EQ(foo.Name)),
			).
			// foo (not having group with name "lab").
			QueryUsers().
			Where(
				user.Not(
					user.Groups.HasWith(group.Name.EQ(lab.Name)),
				),
			).
			// hub.
			QueryGroups().
			OnlyX(ctx).Name,
	)

	t.Log("query path from a client")
	require.Equal(
		bar.Name,
		client.Group.
			// hub.
			Query().
			Where(
				group.Users.HasWith(user.Name.EQ(foo.Name)),
			).
			// foo (not having group with name "lab").
			QueryUsers().
			Where(
				user.Not(
					user.Groups.HasWith(group.Name.EQ(lab.Name)),
				),
			).
			// hub.
			QueryGroups().
			// bar, foo.
			QueryUsers().
			Order(user.Name.Asc()).
			// bar
			FirstX(ctx).Name,
	)
}
