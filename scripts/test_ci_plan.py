import unittest
from pathlib import Path
from ci_plan import classify, NAMES

class RoutingTests(unittest.TestCase):
    def test_go_overrides_targeted_examples(self):
        self.assertEqual(classify(['internal/scan/scan.go','examples/custom-policy/rules.yaml']), ('full',NAMES))
    def test_multiple_examples(self):
        self.assertEqual(classify(['examples/custom-policy/rules.yaml','examples/reusable-module/source/main.tf']), ('examples',['custom-policy','reusable-module']))
    def test_shared_inputs(self):
        for p in ['go.sum','rules/default.yaml','scripts/ci_plan.py','.github/workflows/ci.yml','cmd/exampletest/main.go']:
            self.assertEqual(classify([p])[0], 'full', p)
    def test_documentation(self):
        self.assertEqual(classify(['README.md','examples/custom-policy/README.md']), ('docs',[]))
    def test_results_require_parent_proof(self):
        with self.assertRaises(ValueError): classify(['examples/custom-policy/results/jev/run.json'])
        with self.assertRaises(ValueError): classify(['examples/custom-policy/results/jev/run.json'],True,['reusable-module'])
        self.assertEqual(classify(['examples/custom-policy/results/jev/run.json'],True,['custom-policy']),('results',[]))
    def test_results_mixed_with_docs_rerun_the_owner(self):
        """Documentation must not exempt changed reports from validation."""
        report = 'examples/custom-policy/results/jev/run.json'
        for proven_parent in (False, True):
            for paths in ([report, 'README.md'], ['README.md', report]):
                with self.subTest(proven_parent=proven_parent, paths=paths):
                    self.assertEqual(classify(paths, proven_parent, ['custom-policy']),
                                     ('examples', ['custom-policy']))
    def test_results_mixed_with_other_example_select_both(self):
        """A separate source edit must not hide a report's owning example."""
        paths = ['examples/custom-policy/results/offline/report.json',
                 'examples/reusable-module/source/main.tf']
        for ordered in (paths, list(reversed(paths))):
            self.assertEqual(classify(ordered), ('examples', ['custom-policy', 'reusable-module']))
    def test_shared_inputs_override_mixed_reports_and_docs(self):
        """Shared inputs retain full validation regardless of path order."""
        paths = ['README.md', 'examples/custom-policy/results/jev/run.json', 'go.mod']
        for ordered in (paths, list(reversed(paths))):
            self.assertEqual(classify(ordered), ('full', NAMES))
    def test_example_source_markdown_is_an_example_input(self):
        """Every fixture source file participates in example validation."""
        self.assertEqual(classify(['examples/reusable-module/source/policy.md']),
                         ('examples', ['reusable-module']))
    def test_code_hidden_in_results_is_not_exempt(self):
        self.assertEqual(classify(['examples/custom-policy/results/evil.go'],True,['custom-policy'])[0],'full')
    def test_unknown_input_and_deleted_example(self):
        self.assertEqual(classify(['Makefile'])[0],'full')
        self.assertEqual(classify(['examples/custom-policy/source/main.tf'])[1],['custom-policy'])

class GateTests(unittest.TestCase):
    def test_failed_or_skipped_required_job_blocks(self):
        from ci_gate import validate
        for failed in ['failure', 'cancelled', 'skipped', '']:
            for index in range(4):
                jobs = ['success'] * 4
                jobs[index] = failed
                with self.assertRaises(ValueError): validate('full', *jobs)
        with self.assertRaises(ValueError): validate('examples','success','skipped','skipped','failure')
    def test_selected_routes(self):
        from ci_gate import validate
        validate('full','success','success','success','success')
        validate('examples','success','skipped','skipped','success')
        for mode in ['docs','results']:
            validate(mode,'success','skipped','skipped','skipped')
        with self.assertRaises(ValueError): validate('unknown','success','skipped','skipped','skipped')

class MergeEvidenceTests(unittest.TestCase):
    def test_results_commit_uses_pull_request_validation(self):
        """A skipped pull_request workflow cannot satisfy the recomputed merge check."""
        workflow = Path(__file__).parents[1].joinpath('.github/workflows/ci.yml').read_text()
        publish = workflow.split('name: Save reports to PR branch', 1)[1]
        self.assertNotIn('[skip ci]', publish)
        self.assertNotIn('workflow run ci.yml', publish)
        self.assertIn('chore: refresh example results', publish)

    def test_only_exact_validated_merge_can_receive_result(self):
        from publish_gate import target_matches
        pr = {'head':{'sha':'head'},'base':{'sha':'base'}}
        self.assertTrue(target_matches(pr,'head','base',['base','head']))
        self.assertFalse(target_matches(pr,'other','base',['base','head']))
        self.assertFalse(target_matches(pr,'head','new-base',['base','head']))
        self.assertFalse(target_matches(pr,'head','base',['base','other']))
        self.assertFalse(target_matches(pr,'head','base',['head']))

if __name__ == '__main__': unittest.main()
