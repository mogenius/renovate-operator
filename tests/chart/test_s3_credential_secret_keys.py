import pathlib
import re
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[2]


class S3CredentialSecretKeysTest(unittest.TestCase):
    def test_custom_secret_key_names_are_supported(self):
        template = (ROOT / "charts/renovate-operator/templates/deployment.yaml").read_text()

        self.assertRegex(
            template,
            re.compile(r"key:\s*\{\{\s*\.Values\.s3\.credentials\.accessKeyId\.key\s*\|\s*default\s+\"access-key-id\"\s*\}\}"),
            "custom S3 access key ID secret key is not configurable",
        )
        self.assertRegex(
            template,
            re.compile(r"key:\s*\{\{\s*\.Values\.s3\.credentials\.secretAccessKey\.key\s*\|\s*default\s+\"secret-access-key\"\s*\}\}"),
            "custom S3 secret access key secret key is not configurable",
        )


if __name__ == "__main__":
    unittest.main()
