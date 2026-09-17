package schema

import "strconv"

// Int is a custom GraphQL scalar that maps to Go's int.
//
// It exists because graph-gophers' default Int maps to int32, but our
// domain models use int for IDs. Using this scalar gives us direct int
// without casting in every resolver.
//
// Usage in a schema:
//
//	scalar Int
//	type User {
//	    id: Int!
//	}
//
// The schema must declare `scalar Int` for this to work.
type Int int

// ImplementsGraphQLType tells graph-gophers this scalar handles "Int".
func (Int) ImplementsGraphQLType(name string) bool {
	return name == "Int"
}

// UnmarshalGraphQL parses an incoming GraphQL value into an Int.
//
// Accepts:
//   - int, int32, int64 (Go numeric types)
//   - float64 (JSON number unmarshaling)
//   - string (numeric string)
func (i *Int) UnmarshalGraphQL(input interface{}) error {
	switch v := input.(type) {
	case int:
		*i = Int(v)
	case int32:
		*i = Int(v)
	case int64:
		*i = Int(v)
	case float64:
		*i = Int(int(v))
	case string:
		val, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		*i = Int(val)
	default:
		return nil
	}
	return nil
}

// MarshalGraphQL converts an Int back to a Go value for the response.
func (i Int) MarshalGraphQL() interface{} {
	return int(i)
}
