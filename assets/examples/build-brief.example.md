<!-- A real build brief that passed the checker: the M1 "write the unified
shipment models" step of the Swiftpost track. Kept whole rather than trimmed,
because the useful part is how specific the constraints and the Compare
section are. The format is in references/build-briefs.md. -->

# M1 · Build step — the unified shipment models

**You can start here.** You do not have to read M1's lessons first. Every
sub-problem below names the concept that explains it, so being stuck resolves to
one link rather than a search. Read the lesson at the point it becomes useful —
that is the whole idea (R17).

---

## Goal

Write the shapes the entire shipment feature will speak, in `models/shipment_booking_models/`
in shipping-service. Nothing else exists yet; this is the vocabulary M2's supplier
contract, M4's normalizer, M8's pricing and the API responses will all be built
on.

The hard part is not Go. It is deciding **whose words win**. Swiftpost says
`consignmentId`, `rateId`, `sessionId`, `journeyRef`, and a `priceComponents` array with
a `category` of `"BF"` or `"TAX"`. Cargolink says `CreateLabel`, `ServiceType`, `SessionRef`,
and hands back an opaque blob the browser echoes. Neither vocabulary may reach
anything above the adapter. What you write here is what replaces both.

## What is pinned, and what is yours

Pinned, because something outside you depends on it:

| Pinned | Why |
|---|---|
| The **JSON field names** the tests use (`amount`, `currency`, `base`, `tax`, `total`, `firstName`, `middleName`, `lastName`, `parcels`, `oversized`, `documents`, `optionId`, `origin`, `destination`, `collectionDate`, `serviceType`, `packaging`, `legs`, `recipients`, `rate`) | This is the published API. The frontend will depend on it. |
| The **exported type names** `Money`, `Rate`, `Recipient`, `RecipientCounts`, `CabinClass`, `ShippingOption`, `SearchRequest`, `SearchLeg` | M2's `Supplier` interface signature references them. |
| Three **methods**: `CabinClass.Valid() bool`, `RecipientCounts.Chargeable() int`, `RecipientCounts.Total() int` | Each encodes a rule, not a naming preference — see the constraints. |
| The constants `CabinEconomy`, `CabinBusiness`, `CabinUnknown` | The tests need something to check against. |

Everything else is yours: your Go field names, whether something is a pointer or
a value or a wrapper, how you represent the packaging and recipient-type sets, what
extra types you add, what methods you hang off them, how you split the files.

## Constraints

Stated as the problems they are. How you solve each one is the exercise.

1. **A parcel who gave no middle name and one who gave a blank middle name
   are different facts.** An carrier rejects a blank name differently from an
   absent one. The two must not collapse into the same value — including after a
   round trip through JSON. → `needs: go.zero-values`

2. **A waived fee is a real zero, not a missing value.** A client must be able to
   tell "nothing to pay" from "we didn't say". → `needs: go.json`

3. **Every fee we charge is computed on base + tax and nothing else.** So base and
   tax have to survive separately; one combined total is not enough.
   → `needs: go.structs`, `sd.canonical-model`

4. **The total is whatever the supplier said, never recomputed.** Swiftpost's
   total legitimately differs from base + tax once its convenience fee applies —
   a fee that appears at *hold*, after the search. Deriving the total would
   undercharge on every such booking. → `needs: sd.canonical-model`

5. **An unmapped supplier code must not travel onward as text.** An adapter has to
   be able to tell a packaging it recognised from one it did not. That is what
   `Valid()` is for. → `needs: go.named-types`, `pat.anti-corruption`

6. **A bare string must not be usable where a packaging is expected.** If a raw
   supplier code can be assigned to a packaging without a conversion, the protection
   does not exist. → `needs: go.named-types`

7. **Infants are counted apart from seated recipients.** The visible shipment
   service fee multiplies by parcels + oversized and excludes documents — that is a
   billing rule the Cargolink pricing already follows, not a preference. That is what
   `Chargeable()` and `Total()` are for. → `needs: go.structs`

8. **The id a client sends back is ours and opaque.** Swiftpost's rate id encodes
   route, carrier, timestamps and its own PCC. Publishing it would put a
   supplier's private encoding in our API and leave Cargolink with nothing to fill it
   with at M10. → `needs: sd.canonical-model`, `pat.anti-corruption`

9. **No supplier's vocabulary appears anywhere in the shapes.** Not
   `rateId`, not `SessionRef`, not `CarrierRef`, not a `supplier` field a client could
   set. → `needs: pat.anti-corruption`

10. **A departure date is a date, not an instant.** A departure is local to its
    depot and no supplier accepts a time zone on it.
    → `needs: go.time`

## Break it down

Suggested order. Each is small enough to finish in one sitting.

| # | Sub-problem | Constraints | needs |
|---|---|---|---|
| 1 | The amount type — an amount and its currency | 2 | `go.structs`, `go.json` |
| 2 | Cabin class and recipient type — closed sets a wrong value cannot enter | 5, 6 | `go.named-types` |
| 3 | Recipient counts, with documents separable | 7 | `go.structs` |
| 4 | The recipient — required parts, and the genuinely optional ones | 1 | `go.zero-values`, `go.json` |
| 5 | The rate — base, tax, total, and the per-recipient breakdown | 3, 4 | `go.structs`, `sd.canonical-model` |
| 6 | The search request — legs, dates, counts, packaging | 9, 10 | `go.time`, `pat.anti-corruption` |
| 7 | The shipping option — legs, legs, the opaque id | 8, 9 | `sd.canonical-model` |

Open a lesson when a row stops being obvious, not before.

## Done when

Copy the test file next to your code and make it pass:

```
cp ~/go-learning\_builds\m01-b1\models_contract_test.go ../shipping-service\models\shipment_booking_models\
cd ../shipping-service
go test ./models/shipment_booking_models/
```

Then, as always:

```
go build ./... && go vet ./... && gofmt -l models/shipment_booking_models/
```

The tests assert properties, not your design — they were checked against a
deliberately naive implementation (plain strings, `omitempty` everywhere,
documents counted as seated, a leaked `rateId`) and caught six distinct mistakes.
Passing them is real.

## Not tested — judgement calls you still have to make

Worth knowing which constraints the tests *cannot* check, so you do not mistake a
green run for a finished design:

- **Stops.** Swiftpost sends a stop count alongside the legs; Cargolink sends only the
  legs. Storing both lets them drift. Deriving it needs a method nothing calls
  yet, so nothing tests it.
- **Whether a field belongs here at all.** The tests catch a supplier word that
  leaks; they cannot catch a field that is merely useless.
- **Passport and international fields.** Out of scope for the first slice
  (one parcel, domestic, standard service). Leaving them out entirely is defensible; so is
  declaring them so the international work is not shaped around a struct that
  cannot hold them. Decide on purpose.
- **File layout.** One file or four. No test cares.

## Compare — open only after your tests pass

One worked version is at `~/go-learning\_reference\m01-m03\models_shipment_booking_models\`.

Reading it before you have passed the tests does to this what reading an
exercise's reference solution does: it teaches you nothing and it costs you the
practice. Afterwards it is worth a careful read, because the interesting part is
where it differs from yours.

Three decisions in it worth arguing with rather than accepting:

1. It uses `*string` for the optional middle name. A `HasMiddleName bool`, or a
   small `Optional[T]` generic, would also pass. The pointer is conventional Go
   and it is also the one that panics if you forget to check it.
2. It declares `TripRoundTrip` and `TripMultiCity` although the first slice
   serves neither, on the grounds that they change the *shape* of a request
   rather than one of its fields. You may think that is speculative.
3. It gives `Leg` both a `Duration time.Duration` and a `DurationMin int`,
   one for Go and one for the wire. That is duplication, and whether it earns
   its place is a fair question.
