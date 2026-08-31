package groups

import (
	"errors"
	"testing"
)

func TestMutationValidateNormalizesAndDeduplicates(t *testing.T) {
	request, err := (Mutation{
		Code:        "VIP_GROUP",
		Name:        "VIP",
		Multiplier:  "0.8",
		RPMLimit:    200,
		BillingType: BillingPrepaid,
		Priority:    10,
		ChannelIDs:  []string{" channel-1 ", "channel-1", "channel-2"},
	}).validate()
	if err != nil {
		t.Fatal(err)
	}
	if request.Code != "vip_group" || request.Multiplier != "0.800000" || len(request.ChannelIDs) != 2 || request.Status != StatusActive {
		t.Fatalf("unexpected normalized request: %#v", request)
	}
}

func TestMutationValidateRejectsUnsupportedBillingAndInvalidLimits(t *testing.T) {
	base := Mutation{Code: "standard", Name: "Standard", Multiplier: "1"}
	for _, request := range []Mutation{
		{Code: base.Code, Name: base.Name, Multiplier: "0"},
		{Code: base.Code, Name: base.Name, Multiplier: "1000.000001"},
		{Code: base.Code, Name: base.Name, Multiplier: "1", BillingType: "postpaid"},
		{Code: base.Code, Name: base.Name, Multiplier: "1", RPMLimit: -1},
	} {
		if _, err := request.validate(); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("expected invalid request for %#v, got %v", request, err)
		}
	}
}
