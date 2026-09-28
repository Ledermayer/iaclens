import unittest
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

if __name__ == '__main__': unittest.main()
