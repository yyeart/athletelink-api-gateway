package coreclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
	"uuid"
)

type jsonObject map[string]json.RawMessage

func readObject(r io.Reader) (jsonObject, error) {
	decoder := json.NewDecoder(r)

	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("read JSON: %w", err)
	}

	var extra json.RawMessage
	err := decoder.Decode(&extra)
	switch {
	case errors.Is(err, io.EOF):
		return parseObject(raw)

	case err == nil:
		return nil, ErrMultipleJSONValues

	default:
		return nil, fmt.Errorf("trailing JSON data: %w", err)
	}
}

func parseObject(raw json.RawMessage) (jsonObject, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))

	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if token != json.Delim('{') {
		return nil, errors.New("expected JSON object")
	}

	result := make(jsonObject)

	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}

		key, ok := token.(string)
		if !ok {
			return nil, errors.New("invalid object key")
		}
		if _, exists := result[key]; exists {
			return nil, errors.New("duplicate object key")
		}

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}

		result[key] = value
	}

	token, err = decoder.Token()
	if err != nil {
		return nil, err
	}
	if token != json.Delim('}') {
		return nil, errors.New("invalid object ending")
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected data after object")
	}

	return result, nil
}

func readField[T any](
	obj jsonObject,
	name string,
	destination **T,
	decode func(json.RawMessage) (T, error),
) error {
	raw, exists := obj[name]
	if !exists {
		return nil
	}

	value, err := decode(raw)
	if err != nil {
		return fmt.Errorf("field %s: %w", name, err)
	}

	*destination = &value

	return nil
}

func parseScalar[T any](raw json.RawMessage) (T, error) {
	var result T

	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return result, errors.New("null is not allowed")
	}

	if err := json.Unmarshal(raw, &result); err != nil {
		return result, err
	}

	return result, nil
}

func parseUUID(raw json.RawMessage) (string, error) {
	value, err := parseScalar[string](raw)
	if err != nil {
		return "", err
	}

	if len(value) != 36 ||
		value[8] != '-' ||
		value[13] != '-' ||
		value[18] != '-' ||
		value[23] != '-' {
		return "", errors.New("invalid UUID representation")
	}

	if _, err := uuid.Parse(value); err != nil {
		return "", errors.New("invalid UUID")
	}

	return value, nil
}

func parseTimestamp(raw json.RawMessage) (time.Time, error) {
	value, err := parseScalar[string](raw)
	if err != nil {
		return time.Time{}, err
	}

	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, errors.New("invalid date-time")
	}

	parsed = parsed.UTC()

	if parsed.Year() < 1 || parsed.Year() > 9999 {
		return time.Time{}, errors.New("date-time outside Timestamp range")
	}

	return parsed, nil
}

func parseEnum(allowed ...string) func(json.RawMessage) (string, error) {
	return func(raw json.RawMessage) (string, error) {
		value, err := parseScalar[string](raw)
		if err != nil {
			return "", err
		}

		for _, candidate := range allowed {
			if value == candidate {
				return value, nil
			}
		}

		return "", errors.New("unknown enum type")
	}
}

func parseParticipant(raw json.RawMessage) (Participant, error) {
	var result Participant

	obj, err := parseObject(raw)
	if err != nil {
		return Participant{}, err
	}

	checks := []error{
		readField(obj, "userId", &result.UserID, parseUUID),
		readField(obj, "role", &result.Role, parseEnum("ORGANIZER", "PLAYER")),
		readField(obj, "status", &result.Status,
			parseEnum("PENDING", "ACCEPTED", "KICKED", "LEFT")),
		readField(obj, "joinedAt", &result.JoinedAt, parseTimestamp),
	}

	for _, err := range checks {
		if err != nil {
			return Participant{}, err
		}
	}

	return result, nil
}

func parseParticipants(raw json.RawMessage) (ParticipantList, error) {
	var result ParticipantList

	items, err := parseScalar[[]json.RawMessage](raw)
	if err != nil {
		return result, err
	}

	result.Values = make([]Participant, 0, len(items))

	for idx, item := range items {
		participant, err := parseParticipant(item)
		if err != nil {
			return ParticipantList{}, fmt.Errorf(
				"participant %d: %w", idx, err,
			)
		}

		result.Values = append(result.Values, participant)
	}

	return result, nil
}

func decodeRequestDetails(r io.Reader) (ActivityRequestDetails, error) {
	var result ActivityRequestDetails

	obj, err := readObject(r)
	if err != nil {
		return ActivityRequestDetails{}, err
	}

	checks := []error{
		readField(obj, "id", &result.ID, parseUUID),
		readField(obj, "title", &result.Title, parseScalar[string]),
		readField(obj, "description",
			&result.Description, parseScalar[string]),
		readField(obj, "sportName",
			&result.SportName, parseScalar[string]),
		readField(obj, "maxPlayers",
			&result.MaxPlayers, parseScalar[int32]),
		readField(obj, "currentPlayers",
			&result.CurrentPlayers, parseScalar[int32]),
		readField(obj, "eventDate",
			&result.EventDate, parseTimestamp),
		readField(obj, "addressText",
			&result.AddressText, parseScalar[string]),
		readField(obj, "numberOfRounds",
			&result.NumberOfRounds, parseScalar[int32]),
		readField(obj, "latitude",
			&result.Latitude, parseScalar[float64]),
		readField(obj, "longitude",
			&result.Longitude, parseScalar[float64]),
		readField(obj, "status", &result.Status,
			parseEnum(
				"PLANNED",
				"ACTIVE",
				"CONFIRMATION",
				"COMPLETED",
				"CANCELLED",
				"FROZEN",
			)),
		readField(obj, "registrationOpen",
			&result.RegistrationOpen, parseScalar[bool]),
		readField(obj, "organizerId",
			&result.OrganizerID, parseUUID),
		readField(obj, "participants",
			&result.Participants, parseParticipants),
	}

	for _, err := range checks {
		if err != nil {
			return ActivityRequestDetails{}, err
		}
	}

	return result, nil
}

func parseErrorDetails(raw json.RawMessage) (map[string]string, error) {
	obj, err := parseObject(raw)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(obj))

	for key, rawVal := range obj {
		val, err := parseScalar[string](rawVal)
		if err != nil {
			return nil, errors.New("invalid ApiError details value")
		}

		result[key] = val
	}

	return result, nil
}

func validateAPIError(r io.Reader) error {
	obj, err := readObject(r)
	if err != nil {
		return err
	}

	var code *string
	var message *string
	var details *map[string]string

	checks := []error{
		readField(obj, "code", &code, parseScalar[string]),
		readField(obj, "message", &message, parseScalar[string]),
		readField(obj, "details", &details, parseErrorDetails),
	}

	for _, err := range checks {
		if err != nil {
			return err
		}
	}

	return nil
}

func parseRequestFeed(raw json.RawMessage) (ActivityRequestFeed, error) {
	obj, err := parseObject(raw)
	if err != nil {
		return ActivityRequestFeed{}, err
	}

	var result ActivityRequestFeed
	checks := []error{
		readField(obj, "id", &result.ID, parseUUID),
		readField(obj, "title", &result.Title, parseScalar[string]),
		readField(obj, "sportName", &result.SportName, parseScalar[string]),
		readField(obj, "maxPlayers", &result.MaxPlayers, parseScalar[int32]),
		readField(obj, "currentPlayers", &result.CurrentPlayers, parseScalar[int32]),
		readField(obj, "eventDate", &result.EventDate, parseTimestamp),
		readField(obj, "addressText", &result.AddressText, parseScalar[string]),
		readField(obj, "numberOfRounds", &result.NumberOfRounds, parseScalar[int32]),
		readField(obj, "registrationOpen",
			&result.RegistrationOpen, parseScalar[bool]),
		readField(obj, "latitude", &result.Latitude, parseScalar[float64]),
		readField(obj, "longitude", &result.Longitude, parseScalar[float64]),
	}

	for _, err := range checks {
		if err != nil {
			return ActivityRequestFeed{}, err
		}
	}

	return result, nil
}

func decodeRequestFeeds(r io.Reader) ([]ActivityRequestFeed, error) {
	decoder := json.NewDecoder(r)

	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("read request feeds JSON: %w", err)
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, ErrMultipleJSONValues
		}

		return nil, fmt.Errorf("trailing JSON data: %w", err)
	}

	items, err := parseScalar[[]json.RawMessage](raw)
	if err != nil {
		return nil, fmt.Errorf("expected request feeds array: %w", err)
	}

	result := make([]ActivityRequestFeed, 0, len(items))
	for i, item := range items {
		feed, err := parseRequestFeed(item)
		if err != nil {
			return nil, fmt.Errorf(
				"request feed item %d: %w", i, err,
			)
		}

		result = append(result, feed)
	}

	return result, nil
}

func decodeCreatedRequestID(r io.Reader) (string, error) {
	obj, err := readObject(r)
	if err != nil {
		return "", err
	}

	var id *string
	if err := readField(obj, "requestId", &id, parseUUID); err != nil {
		return "", err
	}

	if id == nil {
		return "", errors.New("missing requestId in Core response")
	}

	return *id, nil
}

func parseRoundUUIDList(raw json.RawMessage) (UUIDList, error) {
	items, err := parseScalar[[]json.RawMessage](raw)
	if err != nil {
		return UUIDList{}, fmt.Errorf("expected UUID array: %w", err)
	}

	result := UUIDList{
		Values: make([]string, 0, len(items)),
	}
	for i, item := range items {
		value, err := parseUUID(item)
		if err != nil {
			return UUIDList{}, fmt.Errorf("UUID at index %d: %w", i, err)
		}

		result.Values = append(result.Values, value)
	}

	return result, nil
}

func parseRoundResult(raw json.RawMessage) (RoundResult, error) {
	obj, err := parseObject(raw)
	if err != nil {
		return RoundResult{}, err
	}
	return parseRoundResultObject(obj)
}

func parseRoundResultObject(obj jsonObject) (RoundResult, error) {
	var result RoundResult
	checks := []error{
		readField(obj, "id", &result.ID, parseUUID),
		readField(obj, "requestId", &result.RequestID, parseUUID),
		readField(obj, "roundNumber", &result.RoundNumber, parseScalar[int32]),
		readField(obj, "winners", &result.Winners, parseRoundUUIDList),
		readField(obj, "losers", &result.Losers, parseRoundUUIDList),
		readField(obj, "recordedBy", &result.RecordedBy, parseUUID),
		readField(obj, "createdAt", &result.CreatedAt, parseTimestamp),
	}

	for _, err := range checks {
		if err != nil {
			return RoundResult{}, err
		}
	}

	return result, nil
}

func decodeRoundResult(r io.Reader) (RoundResult, error) {
	obj, err := readObject(r)
	if err != nil {
		return RoundResult{}, err
	}
	return parseRoundResultObject(obj)
}

func decodeRoundResults(r io.Reader) ([]RoundResult, error) {
	decoder := json.NewDecoder(r)

	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("read round results JSON: %w", err)
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, ErrMultipleJSONValues
		}

		return nil, fmt.Errorf("trailing JSON data: %w", err)
	}

	items, err := parseScalar[[]json.RawMessage](raw)
	if err != nil {
		return nil, fmt.Errorf("expected round results array: %w", err)
	}

	results := make([]RoundResult, 0, len(items))
	for i, item := range items {
		result, err := parseRoundResult(item)
		if err != nil {
			return nil, fmt.Errorf("round result at index %d: %w", i, err)
		}

		results = append(results, result)
	}

	return results, nil
}

func parseSportObject(obj jsonObject) (Sport, error) {
	var result Sport
	checks := []error{
		readField(obj, "id", &result.ID, parseScalar[int64]),
		readField(obj, "name", &result.Name, parseScalar[string]),
		readField(obj, "minPlayers", &result.MinPlayers, parseScalar[int32]),
		readField(obj, "maxPlayers", &result.MaxPlayers, parseScalar[int32]),
	}

	for _, err := range checks {
		if err != nil {
			return Sport{}, err
		}
	}

	if raw, exists := obj["iconUrl"]; exists &&
		!bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		iconURL, err := parseScalar[string](raw)
		if err != nil {
			return Sport{}, fmt.Errorf("field iconUrl: %w", err)
		}

		result.IconURL = &iconURL
	}

	return result, nil
}

func decodeSport(r io.Reader) (Sport, error) {
	obj, err := readObject(r)
	if err != nil {
		return Sport{}, err
	}

	return parseSportObject(obj)
}

func decodeSports(r io.Reader) ([]Sport, error) {
	decoder := json.NewDecoder(r)

	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("read sports: %w", err)
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, ErrMultipleJSONValues
		}

		return nil, fmt.Errorf("trailing JSON data: %w", err)
	}

	items, err := parseScalar[[]json.RawMessage](raw)
	if err != nil {
		return nil, fmt.Errorf("expected sports array: %w", err)
	}

	results := make([]Sport, 0, len(items))
	for i, item := range items {
		obj, err := parseObject(item)
		if err != nil {
			return nil, fmt.Errorf("sport at index %d: %w", i, err)
		}

		sport, err := parseSportObject(obj)
		if err != nil {
			return nil, fmt.Errorf("sport at index %d: %w", i, err)
		}

		results = append(results, sport)
	}

	return results, nil
}
