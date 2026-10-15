"""Тесты Контракта 1: модель TaskMessage и JSON-схема не должны расходиться."""

import json
import unittest
from copy import deepcopy
from datetime import datetime, timedelta, timezone
from pathlib import Path

from jsonschema import Draft202012Validator
from pydantic import ValidationError

from scheduler.messages import PAYLOAD_FIELD, SCHEMA_VERSION, TaskMessage

ROOT = Path(__file__).resolve().parents[2]
SCHEMA = json.loads((ROOT / "contracts" / "task_message.v1.schema.json").read_text("utf-8"))
EXAMPLE = json.loads(
    (ROOT / "contracts" / "examples" / "task_message.v1.example.json").read_text("utf-8")
)
VALIDATOR = Draft202012Validator(SCHEMA)

# Фикстура воркера (команда B). Появится в main после merge feature/worker-core;
# до этого тест пропускается.
WORKER_FIXTURE = ROOT / "worker" / "task" / "testdata" / "task_message_v1.json"


def errors(instance):
    return [e.message for e in VALIDATOR.iter_errors(instance)]


class SchemaItselfTest(unittest.TestCase):
    def test_schema_is_valid_json_schema(self):
        Draft202012Validator.check_schema(SCHEMA)

    def test_example_matches_schema(self):
        self.assertEqual(errors(EXAMPLE), [])

    @unittest.skipUnless(WORKER_FIXTURE.exists(), "в этой ветке ещё нет кода воркера")
    def test_worker_fixture_matches_schema(self):
        """Сообщение, на котором тестируется воркер, обязано проходить нашу схему."""
        fixture = json.loads(WORKER_FIXTURE.read_text("utf-8"))
        self.assertEqual(errors(fixture), [])
        TaskMessage.model_validate(fixture)


class SchemaRejectsBadMessagesTest(unittest.TestCase):
    def mutated(self, **changes):
        msg = deepcopy(EXAMPLE)
        msg.update(changes)
        return msg

    def test_missing_required_field(self):
        for field in SCHEMA["required"]:
            msg = deepcopy(EXAMPLE)
            del msg[field]
            self.assertTrue(errors(msg), f"поле {field} должно быть обязательным")

    def test_unknown_top_level_field(self):
        self.assertTrue(errors(self.mutated(surprise=1)))

    def test_wrong_schema_version(self):
        self.assertTrue(errors(self.mutated(schema_version=2)))

    def test_try_number_starts_at_one(self):
        self.assertTrue(errors(self.mutated(try_number=0)))

    def test_unknown_operator(self):
        self.assertTrue(errors(self.mutated(operator="ruby")))

    def test_timeout_must_be_positive(self):
        self.assertTrue(errors(self.mutated(timeout_sec=0)))

    def test_queued_at_must_be_utc_z(self):
        self.assertTrue(errors(self.mutated(queued_at="2026-09-16T03:00:04+03:00")))

    def test_execution_date_format(self):
        msg = deepcopy(EXAMPLE)
        msg["context"]["execution_date"] = "16.09.2026"
        self.assertTrue(errors(msg))

    def test_xcom_is_optional(self):
        msg = deepcopy(EXAMPLE)
        del msg["context"]["xcom"]
        self.assertEqual(errors(msg), [])


class ModelTest(unittest.TestCase):
    def test_example_parses(self):
        msg = TaskMessage.model_validate(EXAMPLE)
        self.assertEqual(msg.schema_version, SCHEMA_VERSION)
        self.assertEqual(msg.try_number, 2)
        self.assertEqual(
            msg.context.xcom["extract_orders"], "s3://staging/orders_2026-09-16.parquet"
        )

    def test_model_output_satisfies_schema(self):
        msg = TaskMessage.model_validate(EXAMPLE)
        self.assertEqual(errors(msg.to_json_dict()), [])

    def test_empty_xcom_output_satisfies_schema(self):
        data = deepcopy(EXAMPLE)
        del data["context"]["xcom"]
        payload = TaskMessage.model_validate(data).to_json_dict()
        self.assertEqual(errors(payload), [])

    def test_roundtrip_through_stream_fields(self):
        original = TaskMessage.model_validate(EXAMPLE)
        restored = TaskMessage.from_stream_fields(original.to_stream_fields())
        self.assertEqual(restored, original)

    def test_stream_record_has_only_payload_field(self):
        """Воркер читает только payload; других полей в записи быть не должно."""
        fields = TaskMessage.model_validate(EXAMPLE).to_stream_fields()
        self.assertEqual(set(fields), {PAYLOAD_FIELD})
        self.assertIsInstance(fields[PAYLOAD_FIELD], str)
        self.assertEqual(errors(json.loads(fields[PAYLOAD_FIELD])), [])

    def test_queued_at_serialized_with_z_suffix(self):
        msg = TaskMessage.model_validate(EXAMPLE)
        self.assertEqual(msg.to_json_dict()["queued_at"], "2026-09-16T03:00:04Z")

    def test_queued_at_keeps_microseconds(self):
        data = deepcopy(EXAMPLE)
        data["queued_at"] = "2026-09-16T03:00:04.250000Z"
        out = TaskMessage.model_validate(data).to_json_dict()
        self.assertEqual(out["queued_at"], "2026-09-16T03:00:04.25Z")
        self.assertEqual(errors(out), [])

    def test_non_utc_queued_at_is_rejected(self):
        data = deepcopy(EXAMPLE)
        data["queued_at"] = datetime(2026, 9, 16, 6, 0, 4, tzinfo=timezone(timedelta(hours=3)))
        with self.assertRaises(ValidationError):
            TaskMessage.model_validate(data)

    def test_naive_queued_at_is_rejected(self):
        data = deepcopy(EXAMPLE)
        data["queued_at"] = datetime(2026, 9, 16, 3, 0, 4)
        with self.assertRaises(ValidationError):
            TaskMessage.model_validate(data)

    def test_unknown_field_is_rejected_on_producer_side(self):
        data = deepcopy(EXAMPLE)
        data["tryy_number"] = 3
        with self.assertRaises(ValidationError):
            TaskMessage.model_validate(data)

    def test_unsupported_version_in_payload_is_rejected(self):
        data = deepcopy(EXAMPLE)
        data["schema_version"] = 2
        with self.assertRaises(ValidationError):
            TaskMessage.from_stream_fields({PAYLOAD_FIELD: json.dumps(data)})

    def test_serialization_is_deterministic(self):
        msg = TaskMessage.model_validate(EXAMPLE)
        self.assertEqual(msg.to_json(), msg.to_json())

    def test_retry_keeps_task_instance_id(self):
        first = TaskMessage.model_validate({**EXAMPLE, "try_number": 1})
        retry = first.model_copy(update={"try_number": 2})
        self.assertEqual(first.task_instance_id, retry.task_instance_id)
        self.assertNotEqual(first.to_json(), retry.to_json())

    def test_model_and_schema_agree_on_bad_input(self):
        """Всё, что отвергает схема, должна отвергать и модель."""
        bad_messages = []
        for key, value in [
            ("try_number", 0),
            ("operator", "ruby"),
            ("timeout_sec", 0),
            ("schema_version", 2),
            ("dag_id", "bad id with spaces"),
        ]:
            msg = deepcopy(EXAMPLE)
            msg[key] = value
            bad_messages.append(msg)

        for msg in bad_messages:
            self.assertTrue(errors(msg), f"схема должна отвергать {msg}")
            with self.assertRaises(ValidationError, msg=f"модель должна отвергать {msg}"):
                TaskMessage.model_validate(msg)


if __name__ == "__main__":
    unittest.main()
