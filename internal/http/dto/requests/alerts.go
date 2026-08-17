package requests

// UpdateAlertStatus represents the request to update an alert's status.
//
//	@Description	Request body for updating alert status
type UpdateAlertStatus struct {
	Status         string `json:"status" validate:"required,oneof=active resolved acknowledged ignored" example:"acknowledged"`
	AcknowledgedBy string `json:"acknowledged_by" validate:"omitempty,max=255" example:"operator@example.com"`
}
