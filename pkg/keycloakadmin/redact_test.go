package keycloakadmin

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestClientRepresentation_StillDecodesTheSecret(t *testing.T) {
	var clients []*ClientRepresentation
	if err := json.Unmarshal([]byte(`[{"clientId":"monitoring-service","secret":"read-by-the-checks"}]`), &clients); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(clients) != 1 || clients[0].Secret != "read-by-the-checks" {
		t.Fatalf("decoded clients = %+v, want the secret kept for the configuration checks", clients)
	}
}

// withheldClientFields are the ClientRepresentation fields an API caller never
// sees. Every other field must be returned under the same name.
var withheldClientFields = map[string]bool{"secret": true, "attributes": true, "protocolMappers": true}

// A field added to ClientRepresentation has to be classified here: returned by
// RedactClient, or withheld. Nothing new reaches a caller by default.
func TestRedactClient_ReturnsOnlyTheFieldsItLists(t *testing.T) {
	typ := reflect.TypeOf(ClientRepresentation{})
	for i := range typ.NumField() {
		field := typ.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		t.Run(field.Name, func(t *testing.T) {
			client := &ClientRepresentation{}
			setNonZero(t, reflect.ValueOf(client).Elem().Field(i))

			got := jsonObject(t, RedactClient(client))
			_, returned := reflect.TypeOf(RedactedClient{}).FieldByName(field.Name)
			switch {
			case withheldClientFields[name]:
				if _, ok := got[name]; ok {
					t.Errorf("%q is returned, want it withheld", name)
				}
			case !returned:
				t.Errorf("%q is neither returned by RedactedClient nor listed as withheld", name)
			default:
				if want := jsonObject(t, client)[name]; !reflect.DeepEqual(got[name], want) {
					t.Errorf("%q = %v, want %v", name, got[name], want)
				}
			}
		})
	}
}

func TestRedactClient_LeavesOutAttributesAndProtocolMappers(t *testing.T) {
	client := &ClientRepresentation{
		ClientID: "monitoring-service",
		Secret:   "client-secret",
		Attributes: map[string]string{
			"client.secret.rotated":    "previous-secret",
			"saml.signing.private.key": "signing-private-key",
			"webhook.token":            "custom-token",
		},
		ProtocolMappers: []interface{}{map[string]any{"config": map[string]any{"api.key": "mapper-key"}}},
	}
	raw, err := json.Marshal(RedactClient(client))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, value := range []string{"client-secret", "previous-secret", "signing-private-key", "custom-token", "mapper-key"} {
		if strings.Contains(string(raw), value) {
			t.Errorf("redacted client %s carries %q", raw, value)
		}
	}
}

func TestRedactClients_KeepsOrderAndNils(t *testing.T) {
	if got := RedactClients(nil); got != nil {
		t.Errorf("RedactClients(nil) = %v, want nil", got)
	}

	got := RedactClients([]*ClientRepresentation{{ClientID: "a", Secret: "s"}, nil, {ClientID: "b"}})
	if len(got) != 3 || got[0].ClientID != "a" || got[1] != nil || got[2].ClientID != "b" {
		t.Errorf("RedactClients = %+v, want a, nil, b", got)
	}
}

func setNonZero(t *testing.T, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.String:
		v.SetString("value")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int:
		v.SetInt(7)
	case reflect.Interface:
		v.Set(reflect.ValueOf("value"))
	case reflect.Slice:
		elem := reflect.New(v.Type().Elem()).Elem()
		setNonZero(t, elem)
		v.Set(reflect.Append(reflect.MakeSlice(v.Type(), 0, 1), elem))
	case reflect.Map:
		v.Set(reflect.ValueOf(map[string]string{"pkce.code.challenge.method": "S256"}))
	default:
		t.Fatalf("no non-zero value for a %s field", v.Kind())
	}
}

func jsonObject(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return object
}
