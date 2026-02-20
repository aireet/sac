package convert

import (
	"encoding/json"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/models"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func SystemSettingToProto(m *models.SystemSetting) *sacv1.SystemSetting {
	pb := &sacv1.SystemSetting{
		Id:          m.ID,
		Key:         m.Key,
		Description: m.Description,
		CreatedAt:   timestamppb.New(m.CreatedAt),
		UpdatedAt:   timestamppb.New(m.UpdatedAt),
	}
	if m.Value != nil {
		pb.Value = settingValueToProto(m.Value)
	}
	return pb
}

func SystemSettingsToProto(ms []models.SystemSetting) []*sacv1.SystemSetting {
	out := make([]*sacv1.SystemSetting, len(ms))
	for i := range ms {
		out[i] = SystemSettingToProto(&ms[i])
	}
	return out
}

func UserSettingToProto(m *models.UserSetting) *sacv1.UserSetting {
	pb := &sacv1.UserSetting{
		Id:        m.ID,
		UserId:    m.UserID,
		Key:       m.Key,
		CreatedAt: timestamppb.New(m.CreatedAt),
		UpdatedAt: timestamppb.New(m.UpdatedAt),
	}
	if m.Value != nil {
		pb.Value = settingValueToProto(m.Value)
	}
	return pb
}

func UserSettingsToProto(ms []models.UserSetting) []*sacv1.UserSetting {
	out := make([]*sacv1.UserSetting, len(ms))
	for i := range ms {
		out[i] = UserSettingToProto(&ms[i])
	}
	return out
}

// settingValueToProto converts a SettingValue (json.RawMessage) to a protobuf Value.
func settingValueToProto(sv models.SettingValue) *structpb.Value {
	var raw any
	if err := json.Unmarshal([]byte(sv), &raw); err != nil {
		return structpb.NewStringValue(string(sv))
	}
	v, err := structpb.NewValue(raw)
	if err != nil {
		return structpb.NewStringValue(string(sv))
	}
	return v
}

// ProtoValueToSettingValue converts a protobuf Value to SettingValue (json.RawMessage).
func ProtoValueToSettingValue(v *structpb.Value) models.SettingValue {
	if v == nil {
		return nil
	}
	b, err := v.MarshalJSON()
	if err != nil {
		return nil
	}
	return models.SettingValue(b)
}
