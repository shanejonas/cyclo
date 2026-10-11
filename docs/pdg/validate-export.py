"""Validate each compat document in Cyclo's exported array (requires jsonschema)."""
import argparse
import json
from pathlib import Path

from jsonschema import Draft202012Validator, ValidationError

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("export", type=Path)
args = parser.parse_args()
schema = json.loads(Path(__file__).with_name("compat-0.1.0.schema.json").read_text())
Draft202012Validator.check_schema(schema)
validator = Draft202012Validator(schema)
with args.export.open() as stream:
    documents = json.load(stream)
if not isinstance(documents, list):
    parser.error("export must contain a JSON array of function documents")
for index, document in enumerate(documents):
    try:
        validator.validate(document)
    except ValidationError as error:
        raise SystemExit(f"document {index}: {error}") from error
print(f"Validated {len(documents)} compat documents against draft 0.1.0")
