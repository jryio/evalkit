"""Prints the user record the client reads, as JSON."""
import json

def user():
    return {
        "user_name": "Ada",
        "created": "2026-01-05",
        "total": 1200,
        "is_admin": True,
        "addr": "1 Main St",
    }

if __name__ == "__main__":
    print(json.dumps(user()))
